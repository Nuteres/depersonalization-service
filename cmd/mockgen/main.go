package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dep_service/internal/service/inspector"
	"github.com/xuri/excelize/v2"
)

func createDocx(filePath string, bodyXML string) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`
	w, err := zw.Create("[Content_Types].xml")
	if err != nil {
		return err
	}
	w.Write([]byte(contentTypes))

	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`
	w, err = zw.Create("_rels/.rels")
	if err != nil {
		return err
	}
	w.Write([]byte(rels))

	docXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    %s
    <w:sectPr>
      <w:pgSz w:w="11906" w:h="16838"/>
      <w:pgMar w:top="1134" w:right="1134" w:bottom="1134" w:left="1701" w:header="708" w:footer="708" w:gutter="0"/>
    </w:sectPr>
  </w:body>
</w:document>`, bodyXML)
	w, err = zw.Create("word/document.xml")
	if err != nil {
		return err
	}
	w.Write([]byte(docXML))

	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(filePath, buf.Bytes(), 0644)
}

func createOdt(filePath string, bodyXML string) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	mh := &zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	}
	mw, err := zw.CreateHeader(mh)
	if err != nil {
		return err
	}
	mw.Write([]byte("application/vnd.oasis.opendocument.text"))

	manifestXML := `<?xml version="1.0" encoding="UTF-8"?>
<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.2">
  <manifest:file-entry manifest:full-path="/" manifest:version="1.2" manifest:media-type="application/vnd.oasis.opendocument.text"/>
  <manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>
</manifest:manifest>`
	w, err := zw.Create("META-INF/manifest.xml")
	if err != nil {
		return err
	}
	w.Write([]byte(manifestXML))

	contentXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"
 xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0">
 <office:body>
  <office:text>
    %s
  </office:text>
 </office:body>
</office:document-content>`, bodyXML)
	cw, err := zw.CreateHeader(&zip.FileHeader{Name: "content.xml", Method: zip.Deflate})
	if err != nil {
		return err
	}
	cw.Write([]byte(contentXML))

	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(filePath, buf.Bytes(), 0644)
}

func createMultiPagePdf(filePath string, pages [][]string) error {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")

	numPages := len(pages)
	fontObjId := 3 + numPages*2
	totalObjs := fontObjId

	offsets := make([]int, totalObjs+1)

	// 1 0 obj: Catalog
	offsets[1] = b.Len()
	fmt.Fprintf(&b, "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	// 2 0 obj: Pages root
	offsets[2] = b.Len()
	var kids strings.Builder
	for i := 0; i < numPages; i++ {
		pageObjId := 3 + i*2
		fmt.Fprintf(&kids, "%d 0 R ", pageObjId)
	}
	fmt.Fprintf(&b, "2 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n", strings.TrimSpace(kids.String()), numPages)

	// Pages and Content Streams
	for i, lines := range pages {
		pageObjId := 3 + i*2
		contentObjId := 4 + i*2

		offsets[pageObjId] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents %d 0 R /Resources << /Font << /F1 %d 0 R >> >> >>\nendobj\n",
			pageObjId, contentObjId, fontObjId)

		var streamBuf bytes.Buffer
		streamBuf.WriteString("BT\n/F1 9.5 Tf\n45 795 Td\n")
		first := true
		for _, l := range lines {
			if !first {
				streamBuf.WriteString("0 -14.5 Td\n")
			}
			escaped := strings.ReplaceAll(l, `\`, `\\`)
			escaped = strings.ReplaceAll(escaped, `(`, `\(`)
			escaped = strings.ReplaceAll(escaped, `)`, `\)`)
			fmt.Fprintf(&streamBuf, "(%s) Tj\n", escaped)
			first = false
		}
		streamBuf.WriteString("ET\n")
		streamBytes := streamBuf.Bytes()

		offsets[contentObjId] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n<< /Length %d >>\nstream\n", contentObjId, len(streamBytes))
		b.Write(streamBytes)
		b.WriteString("\nendstream\nendobj\n")
	}

	// Font object
	offsets[fontObjId] = b.Len()
	fmt.Fprintf(&b, "%d 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n", fontObjId)

	// xref table
	startXref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", totalObjs+1)
	for i := 1; i <= totalObjs; i++ {
		fmt.Fprintf(&b, "%010d 00000 n \n", offsets[i])
	}

	// trailer
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", totalObjs+1, startXref)

	return os.WriteFile(filePath, b.Bytes(), 0644)
}

func createTxt(filePath string, content string) error {
	return os.WriteFile(filePath, []byte(content), 0644)
}

func createSpreadsheetODS(filePath string, sheetName string, headers []string, rows [][]string) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	mh := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mw, err := zw.CreateHeader(mh)
	if err != nil {
		return err
	}
	mw.Write([]byte("application/vnd.oasis.opendocument.spreadsheet"))

	manifestXML := `<?xml version="1.0" encoding="UTF-8"?>
<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.2">
  <manifest:file-entry manifest:full-path="/" manifest:version="1.2" manifest:media-type="application/vnd.oasis.opendocument.spreadsheet"/>
  <manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>
</manifest:manifest>`
	w, _ := zw.Create("META-INF/manifest.xml")
	w.Write([]byte(manifestXML))

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
 <office:body>
  <office:spreadsheet>`)
	fmt.Fprintf(&sb, `<table:table table:name="%s">`, sheetName)

	sb.WriteString("<table:table-row>")
	for _, h := range headers {
		fmt.Fprintf(&sb, `<table:table-cell office:value-type="string"><text:p>%s</text:p></table:table-cell>`, h)
	}
	sb.WriteString("</table:table-row>")

	for _, r := range rows {
		sb.WriteString("<table:table-row>")
		for _, cell := range r {
			fmt.Fprintf(&sb, `<table:table-cell office:value-type="string"><text:p>%s</text:p></table:table-cell>`, cell)
		}
		sb.WriteString("</table:table-row>")
	}

	sb.WriteString(`</table:table>
  </office:spreadsheet>
 </office:body>
</office:document-content>`)

	cw, err := zw.Create("content.xml")
	if err != nil {
		return err
	}
	cw.Write([]byte(sb.String()))

	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(filePath, buf.Bytes(), 0644)
}

func createSpreadsheetXLSX(filePath string, sheetName string, headers []string, rows [][]string) error {
	f := excelize.NewFile()
	defer f.Close()

	f.SetSheetName("Sheet1", sheetName)

	for colIdx, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
		f.SetCellValue(sheetName, cell, h)
	}

	for rowIdx, row := range rows {
		for colIdx, val := range row {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			f.SetCellValue(sheetName, cell, val)
		}
	}

	return f.SaveAs(filePath)
}

func createSpreadsheetCSV(filePath string, headers []string, rows [][]string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	writer.Comma = ';'
	defer writer.Flush()

	if err := writer.Write(headers); err != nil {
		return err
	}
	return writer.WriteAll(rows)
}

func main() {
	baseDir := "mock_documents"
	_ = os.RemoveAll(baseDir)

	dir1 := filepath.Join(baseDir, "01_trudovoy_dogovor_it")
	dir2 := filepath.Join(baseDir, "02_trudovoy_dogovor_rukovoditel")
	dir3 := filepath.Join(baseDir, "03_soglasie_pdn")
	dir4 := filepath.Join(baseDir, "04_zayavlenie_kompensaciya")
	dir5 := filepath.Join(baseDir, "05_akt_priema_peredachi_tmc")
	dir6 := filepath.Join(baseDir, "06_tablichnye_reestry")

	for _, d := range []string{dir1, dir2, dir3, dir4, dir5, dir6} {
		if err := os.MkdirAll(d, 0755); err != nil {
			panic(err)
		}
	}

	fmt.Println("=======================================================================")
	fmt.Println("🚀 Генерация комплексных документов 2026 года для проверки dep_service")
	fmt.Println("=======================================================================")

	// =========================================================================
	// ДОКУМЕНТ 1: ТРУДОВОЙ ДОГОВОР О ДИСТАНЦИОННОЙ РАБОТЕ (IT-РАЗРАБОТЧИК)
	// =========================================================================
	fmt.Println("\n📄 [1/5] Создание: «Трудовой договор № ТД-2026/048 о дистанционной работе»...")

	doc1DocxBody := `
<w:p><w:pPr><w:jc w:val="center"/><w:spacing w:before="240" w:after="120"/></w:pPr><w:r><w:rPr><w:b/><w:sz w:val="28"/></w:rPr><w:t>ТРУДОВОЙ ДОГОВОР № ТД-2026/048</w:t></w:r></w:p>
<w:p><w:pPr><w:jc w:val="center"/><w:spacing w:after="240"/></w:pPr><w:r><w:rPr><w:i/><w:sz w:val="22"/></w:rPr><w:t>(о дистанционной работе в соответствии с главой 49.1 ТК РФ)</w:t></w:r></w:p>
<w:p><w:pPr><w:jc w:val="both"/><w:spacing w:after="180"/></w:pPr><w:r><w:t>г. Москва                                                                            05 октября 2026 года</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>Общество с ограниченной ответственностью «Инновационные Системы» (ООО «Инновационные Системы»), именуемое в дальнейшем «Работодатель», в лице Генерального директора Сидорова Андрея Васильевича, действующего на основании Устава, с одной стороны, и гражданин Российской Федерации Соколов Дмитрий Михайлович, именуемый в дальнейшем «Работник», с другой стороны (далее совместно именуемые «Стороны»), заключили настоящий трудовой договор (далее – «Договор») о нижеследующем:</w:t></w:r></w:p>
<w:p><w:pPr><w:spacing w:before="180" w:after="60"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>1. ОБЩИЕ ПОЛОЖЕНИЯ</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>1.1. Работодатель поручает, а Работник принимает на себя выполнение трудовых обязанностей в должности «Ведущий инженер-разработчик» в Департаменте информационных технологий.</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>1.2. Работа по настоящему Договору является для Работника основной и выполняется дистанционно на постоянной основе вне места нахождения Работодателя в соответствии со статьей 312.1 ТК РФ.</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>1.3. Местом осуществления трудовой деятельности является фактическое место жительства Работника по адресу: 119048, г. Москва, ул. Усачева, д. 29, корп. 2, кв. 114.</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>1.4. Дата начала работы: 12 октября 2026 года. Настоящий Договор заключен на неопределенный срок. Работнику устанавливается испытательный срок продолжительностью 3 (три) месяца.</w:t></w:r></w:p>
<w:p><w:pPr><w:spacing w:before="180" w:after="60"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>2. ПРАВА И ОБЯЗАННОСТИ СТОРОН</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>2.1. Работник обязуется добросовестно выполнять разработку архитектуры и модулей корпоративного программного обеспечения, соблюдать регламенты информационной безопасности, использовать двухфакторную аутентификацию и шифрование каналов связи.</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>2.2. Взаимодействие Работника и Работодателя осуществляется посредством сети Интернет с использованием корпоративной электронной почты sokolov.dmitry@techcorp.ru и телефонной связи по номеру +7 (916) 234-56-78.</w:t></w:r></w:p>
<w:p><w:pPr><w:spacing w:before="180" w:after="60"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>3. ОПЛАТА ТРУДА, ГАРАНТИИ И КОМПЕНСАЦИИ</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>3.1. За выполнение трудовых обязанностей Работнику устанавливается должностной оклад в размере 320 000 (триста двадцать тысяч) рублей 00 копеек в месяц.</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>3.2. В соответствии со статьей 312.6 ТК РФ Работодатель выплачивает Работнику ежемесячную компенсацию расходов за использование личного компьютерного оборудования, программного обеспечения, электроэнергии и сети Интернет в размере 5 000 (пять тысяч) рублей в месяц.</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>3.3. Заработная плата и компенсационные выплаты перечисляются дважды в месяц (10-го и 25-го числа) в безналичном порядке на банковскую карту МИР 2200 8183 9572 5772 (лицевой счет: 40817810500001234567 в ПАО Сбербанк, БИК: 044525225, к/с: 30101810400000000225).</w:t></w:r></w:p>
<w:p><w:pPr><w:spacing w:before="180" w:after="60"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>4. РЕЖИМ РАБОЧЕГО ВРЕМЕНИ И ОТДЫХА</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>4.1. Работнику устанавливается 40-часовая пятидневная рабочая неделя с понедельника по пятницу с продолжительностью ежедневной работы 8 часов с 10:00 до 19:00 по московскому времени с перерывом для отдыха и питания 1 час.</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>4.2. Работнику предоставляется ежегодный основной оплачиваемый отпуск продолжительностью 28 календарных дней, а также дополнительный оплачиваемый отпуск за ненормированный рабочий день продолжительностью 3 календарных дня.</w:t></w:r></w:p>
<w:p><w:pPr><w:spacing w:before="180" w:after="60"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>5. ДЕПЕРСОНАЛИЗАЦИЯ И ЗАЩИТА ПЕРСОНАЛЬНЫХ ДАННЫХ (152-ФЗ РФ)</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>5.1. Обработка персональных данных Работника осуществляется в строгом соответствии с Федеральным законом № 152-ФЗ «О персональных данных» и Приказом Роскомнадзора от 19.06.2025 № 140.</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>5.2. Работодатель гарантирует применение криптографических методов деперсонализации и деобезличивания данных (AES-256-GCM, PBKDF2) при передаче сведений в корпоративные аналитические сервисы и хранилища без права несанкционированного доступа третьих лиц.</w:t></w:r></w:p>
<w:p><w:pPr><w:spacing w:before="180" w:after="60"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>6. АДРЕСА, БАНКОВСКИЕ РЕКВИЗИТЫ И ПОДПИСИ СТОРОН</w:t></w:r></w:p>
<w:tbl>
  <w:tblPr>
    <w:tblW w:w="9200" w:type="dxa"/>
    <w:tblBorders>
      <w:top w:val="single" w:sz="4" w:space="0" w:color="CCCCCC"/>
      <w:left w:val="single" w:sz="4" w:space="0" w:color="CCCCCC"/>
      <w:bottom w:val="single" w:sz="4" w:space="0" w:color="CCCCCC"/>
      <w:right w:val="single" w:sz="4" w:space="0" w:color="CCCCCC"/>
      <w:insideH w:val="single" w:sz="4" w:space="0" w:color="E0E0E0"/>
      <w:insideV w:val="single" w:sz="4" w:space="0" w:color="E0E0E0"/>
    </w:tblBorders>
  </w:tblPr>
  <w:tr>
    <w:trPr><w:tblHeader/></w:trPr>
    <w:tc><w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="F2F4F7"/><w:tcMar><w:top w:w="120"/><w:bottom w:w="120"/><w:left w:w="180"/><w:right w:w="180"/></w:tcMar></w:tcPr><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>РАБОТОДАТЕЛЬ:</w:t></w:r></w:p></w:tc>
    <w:tc><w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="F2F4F7"/><w:tcMar><w:top w:w="120"/><w:bottom w:w="120"/><w:left w:w="180"/><w:right w:w="180"/></w:tcMar></w:tcPr><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>РАБОТНИК:</w:t></w:r></w:p></w:tc>
  </w:tr>
  <w:tr>
    <w:tc><w:p><w:r><w:t>ООО «Инновационные Системы»&#10;ОГРН: 1157746123456, ИНН: 7704876543&#10;Юридический адрес: 119048, г. Москва, ул. Усачева, д. 29, корп. 2, офис 401&#10;р/с: 40702810938000014521 в ПАО Сбербанк&#10;БИК: 044525225, к/с: 30101810400000000225&#10;Телефон: +7 (495) 789-01-23&#10;Email: corporate@innovatech.ru</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>Соколов Дмитрий Михайлович&#10;Дата рождения: 14.08.1989&#10;Паспорт гражданина РФ: 4515 892341&#10;Выдан: Отделом УФМС района Хамовники г. Москвы 22.09.2015, код подразделения 770-045&#10;СНИЛС: 112-233-445 95&#10;ИНН: 151424725977&#10;Адрес регистрации: 119048, г. Москва, ул. Усачева, д. 29, корп. 2, кв. 114&#10;Телефон: +7 (916) 234-56-78&#10;Email: sokolov.dmitry@techcorp.ru</w:t></w:r></w:p></w:tc>
  </w:tr>
  <w:tr>
    <w:tc><w:p><w:r><w:t>Генеральный директор&#10;__________________ Сидоров А.В.&#10;М.П.</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>Работник&#10;__________________ Соколов Д.М.</w:t></w:r></w:p></w:tc>
  </w:tr>
</w:tbl>
<w:p><w:pPr><w:spacing w:before="180"/></w:pPr><w:r><w:rPr><w:i/><w:sz w:val="20"/></w:rPr><w:t>Второй экземпляр настоящего Договора получил на руки: 05.10.2026 г. ___________ / Соколов Д.М. /</w:t></w:r></w:p>
`

	doc1OdtBody := `
<text:h text:outline-level="1">ТРУДОВОЙ ДОГОВОР № ТД-2026/048</text:h>
<text:p>(о дистанционной работе в соответствии с главой 49.1 ТК РФ)</text:p>
<text:p>г. Москва                                                                            05 октября 2026 года</text:p>
<text:p>Общество с ограниченной ответственностью «Инновационные Системы» (ООО «Инновационные Системы»), именуемое в дальнейшем «Работодатель», в лице Генерального директора Сидорова Андрея Васильевича, действующего на основании Устава, с одной стороны, и гражданин Российской Федерации Соколов Дмитрий Михайлович, именуемый в дальнейшем «Работник», с другой стороны (далее совместно именуемые «Стороны»), заключили настоящий трудовой договор (далее – «Договор») о нижеследующем:</text:p>
<text:h text:outline-level="2">1. ОБЩИЕ ПОЛОЖЕНИЯ</text:h>
<text:p>1.1. Работодатель поручает, а Работник принимает на себя выполнение трудовых обязанностей в должности «Ведущий инженер-разработчик» в Департаменте информационных технологий.</text:p>
<text:p>1.2. Работа по настоящему Договору является для Работника основной и выполняется дистанционно на постоянной основе вне места нахождения Работодателя в соответствии со статьей 312.1 ТК РФ.</text:p>
<text:p>1.3. Местом осуществления трудовой деятельности является фактическое место жительства Работника по адресу: 119048, г. Москва, ул. Усачева, д. 29, корп. 2, кв. 114.</text:p>
<text:p>1.4. Дата начала работы: 12 октября 2026 года. Настоящий Договор заключен на неопределенный срок. Работнику устанавливается испытательный срок продолжительностью 3 (три) месяца.</text:p>
<text:h text:outline-level="2">2. ПРАВА И ОБЯЗАННОСТИ СТОРОН</text:h>
<text:p>2.1. Работник обязуется добросовестно выполнять разработку архитектуры и модулей корпоративного программного обеспечения, соблюдать регламенты информационной безопасности, использовать двухфакторную аутентификацию и шифрование каналов связи.</text:p>
<text:p>2.2. Взаимодействие Работника и Работодателя осуществляется посредством сети Интернет с использованием корпоративной электронной почты sokolov.dmitry@techcorp.ru и телефонной связи по номеру +7 (916) 234-56-78.</text:p>
<text:h text:outline-level="2">3. ОПЛАТА ТРУДА, ГАРАНТИИ И КОМПЕНСАЦИИ</text:h>
<text:p>3.1. За выполнение трудовых обязанностей Работнику устанавливается должностной оклад в размере 320 000 (триста двадцать тысяч) рублей 00 копеек в месяц.</text:p>
<text:p>3.2. В соответствии со статьей 312.6 ТК РФ Работодатель выплачивает Работнику ежемесячную компенсацию расходов за использование личного компьютерного оборудования, программного обеспечения, электроэнергии и сети Интернет в размере 5 000 (пять тысяч) рублей в месяц.</text:p>
<text:p>3.3. Заработная плата и компенсационные выплаты перечисляются дважды в месяц (10-го и 25-го числа) в безналичном порядке на банковскую карту МИР 2200 8183 9572 5772 (лицевой счет: 40817810500001234567 в ПАО Сбербанк, БИК: 044525225, к/с: 30101810400000000225).</text:p>
<text:h text:outline-level="2">4. РЕЖИМ РАБОЧЕГО ВРЕМЕНИ И ОТДЫХА</text:h>
<text:p>4.1. Работнику устанавливается 40-часовая пятидневная рабочая неделя с понедельника по пятницу с продолжительностью ежедневной работы 8 часов с 10:00 до 19:00 по московскому времени с перерывом для отдыха и питания 1 час.</text:p>
<text:p>4.2. Работнику предоставляется ежегодный основной оплачиваемый отпуск продолжительностью 28 календарных дней, а также дополнительный оплачиваемый отпуск за ненормированный рабочий день продолжительностью 3 календарных дня.</text:p>
<text:h text:outline-level="2">5. ДЕПЕРСОНАЛИЗАЦИЯ И ЗАЩИТА ПЕРСОНАЛЬНЫХ ДАННЫХ (152-ФЗ РФ)</text:h>
<text:p>5.1. Обработка персональных данных Работника осуществляется в строгом соответствии с Федеральным законом № 152-ФЗ «О персональных данных» и Приказом Роскомнадзора от 19.06.2025 № 140.</text:p>
<text:p>5.2. Работодатель гарантирует применение криптографических методов деперсонализации и деобезличивания данных (AES-256-GCM, PBKDF2) при передаче сведений в корпоративные аналитические сервисы и хранилища без права несанкционированного доступа третьих лиц.</text:p>
<text:h text:outline-level="2">6. АДРЕСА, БАНКОВСКИЕ РЕКВИЗИТЫ И ПОДПИСИ СТОРОН</text:h>
<table:table table:name="Реквизиты">
  <table:table-header-rows>
    <table:table-row>
      <table:table-cell><text:p>РАБОТОДАТЕЛЬ:</text:p></table:table-cell>
      <table:table-cell><text:p>РАБОТНИК:</text:p></table:table-cell>
    </table:table-row>
  </table:table-header-rows>
  <table:table-row>
    <table:table-cell><text:p>ООО «Инновационные Системы»<text:line-break/>ОГРН: 1157746123456, ИНН: 7704876543<text:line-break/>Юридический адрес: 119048, г. Москва, ул. Усачева, д. 29, корп. 2, офис 401<text:line-break/>р/с: 40702810938000014521 в ПАО Сбербанк<text:line-break/>БИК: 044525225, к/с: 30101810400000000225<text:line-break/>Телефон: +7 (495) 789-01-23<text:line-break/>Email: corporate@innovatech.ru</text:p></table:table-cell>
    <table:table-cell><text:p>Соколов Дмитрий Михайлович<text:line-break/>Дата рождения: 14.08.1989<text:line-break/>Паспорт гражданина РФ: 4515 892341<text:line-break/>Выдан: Отделом УФМС района Хамовники г. Москвы 22.09.2015, код подразделения 770-045<text:line-break/>СНИЛС: 112-233-445 95<text:line-break/>ИНН: 151424725977<text:line-break/>Адрес регистрации: 119048, г. Москва, ул. Усачева, д. 29, корп. 2, кв. 114<text:line-break/>Телефон: +7 (916) 234-56-78<text:line-break/>Email: sokolov.dmitry@techcorp.ru</text:p></table:table-cell>
  </table:table-row>
  <table:table-row>
    <table:table-cell><text:p>Генеральный директор<text:line-break/>__________________ Сидоров А.В.<text:line-break/>М.П.</text:p></table:table-cell>
    <table:table-cell><text:p>Работник<text:line-break/>__________________ Соколов Д.М.</text:p></table:table-cell>
  </table:table-row>
</table:table>
<text:p>Второй экземпляр настоящего Договора получил на руки: 05.10.2026 г. ___________ / Соколов Д.М. /</text:p>
`

	doc1TxtContent := `================================================================================
ТРУДОВОЙ ДОГОВОР № ТД-2026/048
(о дистанционной работе в соответствии с главой 49.1 ТК РФ)
================================================================================
г. Москва                                                    05 октября 2026 года

Общество с ограниченной ответственностью «Инновационные Системы» (ООО «Инновационные Системы»),
в лице Генерального директора Сидорова Андрея Васильевича, с одной стороны, и
гражданин Российской Федерации Соколов Дмитрий Михайлович (Работник), с другой стороны,
заключили настоящий трудовой договор о нижеследующем:

1. ОБЩИЕ ПОЛОЖЕНИЯ
1.1. Работодатель поручает, а Работник принимает на себя обязанности в должности
     «Ведущий инженер-разработчик» в Департаменте информационных технологий.
1.2. Работа выполняется дистанционно вне места нахождения Работодателя (ст. 312.1 ТК РФ).
1.3. Место работы: 119048, г. Москва, ул. Усачева, д. 29, корп. 2, кв. 114.
1.4. Дата начала работы: 12 октября 2026 года. Испытательный срок: 3 (три) месяца.

2. ПРАВА И ОБЯЗАННОСТИ СТОРОН
2.1. Работник обязуется добросовестно выполнять разработку архитектуры корпоративного ПО,
     соблюдать регламенты информационной безопасности, использовать двухфакторную аутентификацию.
2.2. Взаимодействие осуществляется через корпоративную почту sokolov.dmitry@techcorp.ru
     и телефонную связь по номеру +7 (916) 234-56-78.

3. ОПЛАТА ТРУДА И СОЦИАЛЬНЫЕ ГАРАНТИИ
3.1. Должностной оклад: 320 000 (триста двадцать тысяч) рублей в месяц.
3.2. Ежемесячная компенсация за оборудование и связь (ст. 312.6 ТК РФ): 5 000 рублей.
3.3. Выплата на карту МИР: 2200 8183 9572 5772.
     Лицевой счет: 40817810500001234567 в ПАО Сбербанк, БИК: 044525225.

4. РЕЖИМ РАБОЧЕГО ВРЕМЕНИ И ОТДЫХА
4.1. Работнику устанавливается 40-часовая пятидневная рабочая неделя с 10:00 до 19:00.
4.2. Ежегодный основной оплачиваемый отпуск 28 календарных дней + 3 дня за ненормированный день.

5. ДЕПЕРСОНАЛИЗАЦИЯ И ЗАЩИТА ПЕРСОНАЛЬНЫХ ДАННЫХ (152-ФЗ РФ)
5.1. Обработка данных регулируется 152-ФЗ РФ и Приказом Роскомнадзора от 19.06.2025 № 140.
5.2. Работодатель применяет сертифицированные алгоритмы деперсонализации AES-256-GCM.

6. АДРЕСА И РЕКВИЗИТЫ СТОРОН
РАБОТОДАТЕЛЬ:
  ООО «Инновационные Системы», ОГРН: 1157746123456, ИНН: 7704876543
  119048, г. Москва, ул. Усачева, д. 29, корп. 2, офис 401
  Телефон: +7 (495) 789-01-23, email: corporate@innovatech.ru
  Генеральный директор: Сидоров А.В. __________________

РАБОТНИК:
  ФИО: Соколов Дмитрий Михайлович
  Дата рождения: 14.08.1989
  Паспорт гражданина РФ: 4515 892341, выдан 22.09.2015, код подразделения 770-045
  СНИЛС: 112-233-445 95
  ИНН: 151424725977
  Адрес регистрации: 119048, г. Москва, ул. Усачева, д. 29, корп. 2, кв. 114
  Телефон: +7 (916) 234-56-78
  Email: sokolov.dmitry@techcorp.ru
  Подпись: Соколов Д.М. __________________

Второй экземпляр договора получен на руки: 05.10.2026 г. ___________ / Соколов Д.М. /
`

	doc1PdfPages := [][]string{
		{
			"ТРУДОВОЙ ДОГОВОР № ТД-2026/048 (Страница 1 из 2)",
			"о дистанционной работе в соответствии с главой 49.1 ТК РФ",
			"г. Москва, дата заключения: 05 октября 2026 года",
			"Работодатель: ООО Инновационные Системы в лице директора Сидорова Андрея Васильевича",
			"Работник: гражданин РФ Соколов Дмитрий Михайлович",
			"1. Предмет договора: должность Ведущий инженер-разработчик IT-департамента",
			"2. Дистанционный характер работы: 119048, г. Москва, ул. Усачева, д. 29, корп. 2, кв. 114",
			"3. Дата начала выполнения обязанностей: 12 октября 2026 года",
			"4. Оплата труда: должностной оклад 320 000 рублей в месяц",
			"5. Компенсация за интернет и электроэнергию (ст. 312.6 ТК РФ): 5 000 рублей в месяц",
			"6. Безналичное перечисление на банковскую карту МИР: 2200 8183 9572 5772",
			"7. Лицевой счет Работника: 40817810500001234567 в ПАО Сбербанк",
			"8. Режим работы: 40 часов в неделю, пятидневная рабочая неделя",
			"9. Защита ПДн по 152-ФЗ РФ и Приказу Роскомнадзора от 19.06.2025 № 140",
		},
		{
			"ТРУДОВОЙ ДОГОВОР № ТД-2026/048 (Страница 2 из 2)",
			"РЕКВИЗИТЫ И ПОДПИСИ СТОРОН:",
			"Работодатель: ООО Инновационные Системы, ИНН 7704876543, тел: +7 (495) 789-01-23",
			"Работник: Соколов Дмитрий Михайлович",
			"Дата рождения: 14.08.1989",
			"Паспорт гражданина РФ: 4515 892341, выдан 22.09.2015, код подразделения 770-045",
			"СНИЛС: 112-233-445 95",
			"ИНН: 151424725977",
			"Адрес регистрации: 119048, г. Москва, ул. Усачева, д. 29, корп. 2, кв. 114",
			"Контактный телефон: +7 (916) 234-56-78",
			"Электронная почта: sokolov.dmitry@techcorp.ru",
			"Реквизиты карты МИР: 2200 8183 9572 5772, счет 40817810500001234567",
			"Подписи: Генеральный директор Сидоров А.В. / Работник Соколов Д.М.",
			"Экземпляр договора получен Работником: 05.10.2026 г. Соколов Д.М.",
		},
	}

	createDocx(filepath.Join(dir1, "01_trudovoy_dogovor_it.docx"), doc1DocxBody)
	createOdt(filepath.Join(dir1, "01_trudovoy_dogovor_it.odt"), doc1OdtBody)
	createTxt(filepath.Join(dir1, "01_trudovoy_dogovor_it.txt"), doc1TxtContent)
	createMultiPagePdf(filepath.Join(dir1, "01_trudovoy_dogovor_it.pdf"), doc1PdfPages)

	// =========================================================================
	// ДОКУМЕНТ 2: ТРУДОВОЙ ДОГОВОР С КОММЕРЧЕСКИМ ДИРЕКТОРОМ (РУКОВОДИТЕЛЬ)
	// =========================================================================
	fmt.Println("\n📄 [2/5] Создание: «Трудовой договор № ТД-2026/012 с коммерческим директором»...")

	doc2DocxBody := `
<w:p><w:pPr><w:jc w:val="center"/><w:spacing w:before="240" w:after="120"/></w:pPr><w:r><w:rPr><w:b/><w:sz w:val="28"/></w:rPr><w:t>ТРУДОВОЙ ДОГОВОР № ТД-2026/012</w:t></w:r></w:p>
<w:p><w:pPr><w:jc w:val="center"/><w:spacing w:after="240"/></w:pPr><w:r><w:rPr><w:i/><w:sz w:val="22"/></w:rPr><w:t>(с руководящим работником организации)</w:t></w:r></w:p>
<w:p><w:pPr><w:jc w:val="both"/><w:spacing w:after="180"/></w:pPr><w:r><w:t>г. Москва                                                                            01 сентября 2026 года</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>Общество с ограниченной ответственностью «Глобал Логистикс» (ООО «Глобал Логистикс»), именуемое в дальнейшем «Работодатель», в лице Генерального директора Воронова Сергея Алексеевича, с одной стороны, и гражданин Российской Федерации Белов Максим Александрович, именуемый в дальнейшем «Работник», с другой стороны, заключили настоящий договор о нижеследующем:</w:t></w:r></w:p>
<w:p><w:pPr><w:spacing w:before="180" w:after="60"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>1. ПРЕДМЕТ ДОГОВОРА И ДОЛЖНОСТНЫЕ ОБЯЗАННОСТИ</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>1.1. Работник назначается на руководящую должность «Коммерческий директор» компании с подчинением непосредственно Генеральному директору.</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>1.2. Место работы: Головной офис ООО «Глобал Логистикс», 125047, г. Москва, ул. 1-я Тверская-Ямская, д. 18, стр. 1. Дата начала работы: 08 сентября 2026 года.</w:t></w:r></w:p>
<w:p><w:pPr><w:spacing w:before="180" w:after="60"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>2. УСЛОВИЯ ОПЛАТЫ ТРУДА И ПРЕМИРОВАНИЕ</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>2.1. Должностной оклад Работника устанавливается в размере 450 000 (четыреста пятьдесят тысяч) рублей в месяц с выплатой ежеквартальной премии за выполнение целевых показателей коммерческой выручки (KPI).</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>2.2. Заработная плата выплачивается на банковскую карту МИР 2200 5882 8887 8927 (расчетный счет: 40817810800003412567 в Банк ВТБ (ПАО), БИК: 044525745).</w:t></w:r></w:p>
<w:p><w:pPr><w:spacing w:before="180" w:after="60"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>3. РЕЖИМ РАБОТЫ И СОЦИАЛЬНЫЙ ПАКЕТ</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>3.1. В связи со спецификой руководящей деятельности Работнику устанавливается ненормированный рабочий день с предоставлением дополнительного оплачиваемого отпуска в количестве 5 календарных дней.</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>3.2. Работодатель предоставляет Работнику служебный автомобиль бизнес-класса, оплачивает представительские расходы и сотовую связь по номеру +7 (925) 444-55-66.</w:t></w:r></w:p>
<w:p><w:pPr><w:spacing w:before="180" w:after="60"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>4. РЕКВИЗИТЫ И ПОДПИСИ СТОРОН</w:t></w:r></w:p>
<w:tbl>
  <w:tblPr>
    <w:tblW w:w="9200" w:type="dxa"/>
    <w:tblBorders>
      <w:top w:val="single" w:sz="4" w:space="0" w:color="CCCCCC"/>
      <w:left w:val="single" w:sz="4" w:space="0" w:color="CCCCCC"/>
      <w:bottom w:val="single" w:sz="4" w:space="0" w:color="CCCCCC"/>
      <w:right w:val="single" w:sz="4" w:space="0" w:color="CCCCCC"/>
    </w:tblBorders>
  </w:tblPr>
  <w:tr>
    <w:trPr><w:tblHeader/></w:trPr>
    <w:tc><w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="F2F4F7"/><w:tcMar><w:top w:w="120"/><w:bottom w:w="120"/><w:left w:w="180"/><w:right w:w="180"/></w:tcMar></w:tcPr><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>РАБОТОДАТЕЛЬ:</w:t></w:r></w:p></w:tc>
    <w:tc><w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="F2F4F7"/><w:tcMar><w:top w:w="120"/><w:bottom w:w="120"/><w:left w:w="180"/><w:right w:w="180"/></w:tcMar></w:tcPr><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>РАБОТНИК:</w:t></w:r></w:p></w:tc>
  </w:tr>
  <w:tr>
    <w:tc><w:p><w:r><w:t>ООО «Глобал Логистикс»&#10;ОГРН: 1187746981240, ИНН: 7710982341&#10;125047, г. Москва, ул. 1-я Тверская-Ямская, д. 18, стр. 1&#10;р/с: 40702810400000098214 в Банк ВТБ (ПАО)&#10;Генеральный директор: Воронов С.А. __________________</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>Белов Максим Александрович&#10;Дата рождения: 18.06.1982&#10;Паспорт гражданина РФ: 4516 712398&#10;Выдан: ОУФМС по району Тверской г. Москвы 18.04.2016, код подразделения 770-018&#10;СНИЛС: 182-287-392 95&#10;ИНН: 921793189287&#10;Адрес регистрации: 125047, г. Москва, ул. 1-я Тверская-Ямская, д. 18, кв. 34&#10;Телефон: +7 (925) 444-55-66&#10;Email: belov.m@globallogistics.ru&#10;Подпись: Белов М.А. __________________</w:t></w:r></w:p></w:tc>
  </w:tr>
</w:tbl>
`

	doc2OdtBody := `
<text:h text:outline-level="1">ТРУДОВОЙ ДОГОВОР № ТД-2026/012</text:h>
<text:p>(с руководящим работником организации)</text:p>
<text:p>г. Москва                                                                            01 сентября 2026 года</text:p>
<text:p>Общество с ограниченной ответственностью «Глобал Логистикс» (ООО «Глобал Логистикс»), именуемое в дальнейшем «Работодатель», в лице Генерального директора Воронова Сергея Алексеевича, с одной стороны, и гражданин Российской Федерации Белов Максим Александрович, именуемый в дальнейшем «Работник», с другой стороны, заключили настоящий договор о нижеследующем:</text:p>
<text:h text:outline-level="2">1. ПРЕДМЕТ ДОГОВОРА И ДОЛЖНОСТНЫЕ ОБЯЗАННОСТИ</text:h>
<text:p>1.1. Работник назначается на руководящую должность «Коммерческий директор» компании с подчинением непосредственно Генеральному директору.</text:p>
<text:p>1.2. Место работы: Головной офис ООО «Глобал Логистикс», 125047, г. Москва, ул. 1-я Тверская-Ямская, д. 18, стр. 1. Дата начала работы: 08 сентября 2026 года.</text:p>
<text:h text:outline-level="2">2. УСЛОВИЯ ОПЛАТЫ ТРУДА И ПРЕМИРОВАНИЕ</text:h>
<text:p>2.1. Должностной оклад Работника устанавливается в размере 450 000 (четыреста пятьдесят тысяч) рублей в месяц с выплатой ежеквартальной премии за выполнение целевых показателей коммерческой выручки (KPI).</text:p>
<text:p>2.2. Заработная плата выплачивается на банковскую карту МИР 2200 5882 8887 8927 (расчетный счет: 40817810800003412567 в Банк ВТБ (ПАО), БИК: 044525745).</text:p>
<text:h text:outline-level="2">3. РЕЖИМ РАБОТЫ И СОЦИАЛЬНЫЙ ПАКЕТ</text:h>
<text:p>3.1. В связи со спецификой руководящей деятельности Работнику устанавливается ненормированный рабочий день с предоставлением дополнительного оплачиваемого отпуска в количестве 5 календарных дней.</text:p>
<text:p>3.2. Работодатель предоставляет Работнику служебный автомобиль бизнес-класса, оплачивает представительские расходы и сотовую связь по номеру +7 (925) 444-55-66.</text:p>
<text:h text:outline-level="2">4. РЕКВИЗИТЫ И ПОДПИСИ СТОРОН</text:h>
<table:table table:name="Реквизиты">
  <table:table-header-rows>
    <table:table-row>
      <table:table-cell><text:p>РАБОТОДАТЕЛЬ:</text:p></table:table-cell>
      <table:table-cell><text:p>РАБОТНИК:</text:p></table:table-cell>
    </table:table-row>
  </table:table-header-rows>
  <table:table-row>
    <table:table-cell><text:p>ООО «Глобал Логистикс»<text:line-break/>ОГРН: 1187746981240, ИНН: 7710982341<text:line-break/>125047, г. Москва, ул. 1-я Тверская-Ямская, д. 18, стр. 1<text:line-break/>р/с: 40702810400000098214 в Банк ВТБ (ПАО)<text:line-break/>Генеральный директор: Воронов С.А. __________________</text:p></table:table-cell>
    <table:table-cell><text:p>Белов Максим Александрович<text:line-break/>Дата рождения: 18.06.1982<text:line-break/>Паспорт гражданина РФ: 4516 712398<text:line-break/>Выдан: ОУФМС по району Тверской г. Москвы 18.04.2016, код подразделения 770-018<text:line-break/>СНИЛС: 182-287-392 95<text:line-break/>ИНН: 921793189287<text:line-break/>Адрес регистрации: 125047, г. Москва, ул. 1-я Тверская-Ямская, д. 18, кв. 34<text:line-break/>Телефон: +7 (925) 444-55-66<text:line-break/>Email: belov.m@globallogistics.ru<text:line-break/>Подпись: Белов М.А. __________________</text:p></table:table-cell>
  </table:table-row>
</table:table>
`

	doc2TxtContent := `ТРУДОВОЙ ДОГОВОР № ТД-2026/012
(с руководящим работником организации)
г. Москва                                                    01 сентября 2026 года

Общество с ограниченной ответственностью «Глобал Логистикс» (ООО «Глобал Логистикс»),
в лице Генерального директора Воронова Сергея Алексеевича, с одной стороны, и
гражданин Российской Федерации Белов Максим Александрович (Работник), с другой стороны,
заключили настоящий договор о нижеследующем:

1. ПРЕДМЕТ ДОГОВОРА И ДОЛЖНОСТНЫЕ ОБЯЗАННОСТИ
1.1. Работник назначается на руководящую должность «Коммерческий директор» компании.
1.2. Место работы: Головной офис ООО «Глобал Логистикс», 125047, г. Москва, ул. 1-я Тверская-Ямская, д. 18, стр. 1.
     Дата начала работы: 08 сентября 2026 года.

2. УСЛОВИЯ ОПЛАТЫ ТРУДА И ПРЕМИРОВАНИЕ
2.1. Должностной оклад: 450 000 (четыреста пятьдесят тысяч) рублей в месяц.
2.2. Заработная плата выплачивается на банковскую карту МИР 2200 5882 8887 8927
     (расчетный счет: 40817810800003412567 в Банк ВТБ (ПАО), БИК: 044525745).

3. РЕЖИМ РАБОТЫ И СОЦИАЛЬНЫЙ ПАКЕТ
3.1. Ненормированный рабочий день с предоставлением дополнительного отпуска 5 дней.
3.2. Служебный автомобиль бизнес-класса, оплата сотовой связи по номеру +7 (925) 444-55-66.

4. РЕКВИЗИТЫ И ПОДПИСИ СТОРОН
РАБОТОДАТЕЛЬ:
  ООО «Глобал Логистикс», ОГРН: 1187746981240, ИНН: 7710982341
  125047, г. Москва, ул. 1-я Тверская-Ямская, д. 18, стр. 1
  р/с: 40702810400000098214 в Банк ВТБ (ПАО)
  Генеральный директор: Воронов С.А. __________________

РАБОТНИК:
  ФИО: Белов Максим Александрович
  Дата рождения: 18.06.1982
  Паспорт гражданина РФ: 4516 712398, выдан ОУФМС по району Тверской г. Москвы 18.04.2016, код подразделения 770-018
  СНИЛС: 182-287-392 95
  ИНН: 921793189287
  Адрес регистрации: 125047, г. Москва, ул. 1-я Тверская-Ямская, д. 18, кв. 34
  Телефон: +7 (925) 444-55-66
  Email: belov.m@globallogistics.ru
  Подпись: Белов М.А. __________________
`

	doc2PdfPages := [][]string{
		{
			"ТРУДОВОЙ ДОГОВОР № ТД-2026/012 (Страница 1 из 2)",
			"г. Москва, дата заключения: 01 сентября 2026 года",
			"Работодатель: ООО Глобал Логистикс, генеральный директор Воронов Сергей Алексеевич",
			"Работник: гражданин РФ Белов Максим Александрович",
			"Должность: Коммерческий директор компании",
			"Место работы: 125047, г. Москва, ул. 1-я Тверская-Ямская, д. 18, стр. 1",
			"Дата начала работы: 08 сентября 2026 года",
			"Должностной оклад: 450 000 рублей в месяц + квартальные бонусы по KPI",
			"Банковская карта МИР: 2200 5882 8887 8927, счет 40817810800003412567 в Банк ВТБ",
			"Ненормированный рабочий день, предоставление служебного автомобиля",
		},
		{
			"ТРУДОВОЙ ДОГОВОР № ТД-2026/012 (Страница 2 из 2)",
			"РЕКВИЗИТЫ СТОРОН:",
			"Работодатель: ООО Глобал Логистикс, ИНН 7710982341, тел: +7 (495) 222-33-44",
			"Работник: Белов Максим Александрович",
			"Дата рождения: 18.06.1982",
			"Паспорт гражданина РФ: 4516 712398, выдан 18.04.2016, код подразделения 770-018",
			"СНИЛС: 182-287-392 95",
			"ИНН: 921793189287",
			"Адрес регистрации: 125047, г. Москва, ул. 1-я Тверская-Ямская, д. 18, кв. 34",
			"Телефон: +7 (925) 444-55-66, email: belov.m@globallogistics.ru",
			"Подписи сторон: Воронов С.А. / Белов М.А.",
		},
	}

	createDocx(filepath.Join(dir2, "02_trudovoy_dogovor_rukovoditel.docx"), doc2DocxBody)
	createOdt(filepath.Join(dir2, "02_trudovoy_dogovor_rukovoditel.odt"), doc2OdtBody)
	createTxt(filepath.Join(dir2, "02_trudovoy_dogovor_rukovoditel.txt"), doc2TxtContent)
	createMultiPagePdf(filepath.Join(dir2, "02_trudovoy_dogovor_rukovoditel.pdf"), doc2PdfPages)

	// =========================================================================
	// ДОКУМЕНТ 3: СОГЛАСИЕ НА ОБРАБОТКУ ПЕРСОНАЛЬНЫХ ДАННЫХ (152-ФЗ / 140 РКН)
	// =========================================================================
	fmt.Println("\n📄 [3/5] Создание: «Согласие на обработку персональных данных (Морозова Е.С.)»...")

	doc3DocxBody := `
<w:p><w:pPr><w:jc w:val="center"/><w:spacing w:before="240" w:after="120"/></w:pPr><w:r><w:rPr><w:b/><w:sz w:val="28"/></w:rPr><w:t>СОГЛАСИЕ НА ОБРАБОТКУ ПЕРСОНАЛЬНЫХ ДАННЫХ</w:t></w:r></w:p>
<w:p><w:pPr><w:jc w:val="center"/><w:spacing w:after="240"/></w:pPr><w:r><w:rPr><w:i/><w:sz w:val="20"/></w:rPr><w:t>(в соответствии с Федеральным законом № 152-ФЗ и Приказом Роскомнадзора от 19.06.2025 № 140)</w:t></w:r></w:p>
<w:p><w:pPr><w:jc w:val="both"/><w:spacing w:after="180"/></w:pPr><w:r><w:t>г. Санкт-Петербург                                                                    12 октября 2026 года</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>Я, нижеподписавшаяся гражданка Российской Федерации Морозова Елена Сергеевна, дата рождения: 25.04.1994, документ, удостоверяющий личность: паспорт гражданина РФ серия 4014 номер 382910, выдан ТП № 15 ОУФМС России по Санкт-Петербургу и Ленинградской обл. в Приморском р-не г. Санкт-Петербурга 10.05.2014, код подразделения: 780-015; зарегистрированная по месту жительства по адресу: 197374, г. Санкт-Петербург, ул. Савушкина, д. 126, кв. 48; страховой номер индивидуального лицевого счета (СНИЛС): 200-111-222 45; идентификационный номер налогоплательщика (ИНН): 849949023870; контактный телефон: +7 (921) 987-65-43; адрес электронной почты: morozova.elena@datalab.spb.ru,</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>настоящим в соответствии со статьей 9 Федерального закона от 27.07.2006 № 152-ФЗ «О персональных данных» свободно, своей волей и в своем интересе даю безусловное согласие Обществу с ограниченной ответственностью «Аналитика Данных Северо-Запад» (ОГРН: 1187847123980, ИНН: 7814987650, адрес местонахождения: 197374, г. Санкт-Петербург, ул. Савушкина, д. 126, лит. Б, оф. 302) на обработку моих персональных данных со следующими условиями:</w:t></w:r></w:p>
<w:p><w:pPr><w:spacing w:before="180" w:after="60"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>1. ПЕРЕЧЕНЬ ПЕРСОНАЛЬНЫХ ДАННЫХ</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>Обработке подлежат: фамилия, имя, отчество; число, месяц и год рождения; реквизиты паспорта РФ; адрес постоянной регистрации и фактического проживания; СНИЛС; ИНН; номера телефонов и адреса электронной почты; сведения о трудовой деятельности и квалификации; банковские реквизиты: номер карты МИР 2200 3979 6984 3733, расчетный счет 40817810300009876543 в Банк ВТБ (ПАО).</w:t></w:r></w:p>
<w:p><w:pPr><w:spacing w:before="180" w:after="60"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>2. ЦЕЛИ ОБРАБОТКИ И ДЕПЕРСОНАЛИЗАЦИЯ ДАННЫХ</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>2.1. Персональные данные обрабатываются в целях заключения и исполнения трудового договора, исчисления налоговых платежей в ФНС РФ, начисления выплат Социального фонда России (СФР) и ведения кадрового делопроизводства.</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>2.2. Настоящим выражаю согласие на осуществление Оператором статистической и семантической деперсонализации (обезличивания) моих персональных данных в соответствии с методиками и требованиями Приказа Роскомнадзора от 19.06.2025 № 140.</w:t></w:r></w:p>
<w:p><w:pPr><w:spacing w:before="180" w:after="60"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>3. СРОК ДЕЙСТВИЯ И ПОРЯДОК ОТЗЫВА</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>3.1. Настоящее согласие действует со дня его подписания в течение всего срока действия трудового договора и в течение 5 (пяти) лет после его прекращения в соответствии со сроками архивного хранения кадровых документов.</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>3.2. Настоящее согласие может быть отозвано письменным заявлением, направленным в адрес Оператора заказным почтовым отправлением с описью вложения.</w:t></w:r></w:p>
<w:p><w:pPr><w:spacing w:before="240"/></w:pPr><w:r><w:t>Субъект персональных данных: ____________________ / Морозова Е.С. /&#10;Дата подписания: 12 октября 2026 года</w:t></w:r></w:p>
`

	doc3OdtBody := `
<text:h text:outline-level="1">СОГЛАСИЕ НА ОБРАБОТКУ ПЕРСОНАЛЬНЫХ ДАННЫХ</text:h>
<text:p>(в соответствии с Федеральным законом № 152-ФЗ и Приказом Роскомнадзора от 19.06.2025 № 140)</text:p>
<text:p>г. Санкт-Петербург                                                                    12 октября 2026 года</text:p>
<text:p>Я, нижеподписавшаяся гражданка Российской Федерации Морозова Елена Сергеевна, дата рождения: 25.04.1994, документ, удостоверяющий личность: паспорт гражданина РФ серия 4014 номер 382910, выдан ТП № 15 ОУФМС России по Санкт-Петербургу и Ленинградской обл. в Приморском р-не г. Санкт-Петербурга 10.05.2014, код подразделения: 780-015; зарегистрированная по месту жительства по адресу: 197374, г. Санкт-Петербург, ул. Савушкина, д. 126, кв. 48; страховой номер индивидуального лицевого счета (СНИЛС): 200-111-222 45; идентификационный номер налогоплательщика (ИНН): 849949023870; контактный телефон: +7 (921) 987-65-43; адрес электронной почты: morozova.elena@datalab.spb.ru,</text:p>
<text:p>настоящим в соответствии со статьей 9 Федерального закона от 27.07.2006 № 152-ФЗ «О персональных данных» свободно, своей волей и в своем интересе даю безусловное согласие Обществу с ограниченной ответственностью «Аналитика Данных Северо-Запад» (ОГРН: 1187847123980, ИНН: 7814987650, адрес местонахождения: 197374, г. Санкт-Петербург, ул. Савушкина, д. 126, лит. Б, оф. 302) на обработку моих персональных данных со следующими условиями:</text:p>
<text:h text:outline-level="2">1. ПЕРЕЧЕНЬ ПЕРСОНАЛЬНЫХ ДАННЫХ</text:h>
<text:p>Обработке подлежат: фамилия, имя, отчество; число, месяц и год рождения; реквизиты паспорта РФ; адрес постоянной регистрации и фактического проживания; СНИЛС; ИНН; номера телефонов и адреса электронной почты; сведения о трудовой деятельности и квалификации; банковские реквизиты: номер карты МИР 2200 3979 6984 3733, расчетный счет 40817810300009876543 в Банк ВТБ (ПАО).</text:p>
<text:h text:outline-level="2">2. ЦЕЛИ ОБРАБОТКИ И ДЕПЕРСОНАЛИЗАЦИЯ ДАННЫХ</text:h>
<text:p>2.1. Персональные данные обрабатываются в целях заключения и исполнения трудового договора, исчисления налоговых платежей в ФНС РФ, начисления выплат Социального фонда России (СФР) и ведения кадрового делопроизводства.</text:p>
<text:p>2.2. Настоящим выражаю согласие на осуществление Оператором статистической и семантической деперсонализации (обезличивания) моих персональных данных в соответствии с методиками и требованиями Приказа Роскомнадзора от 19.06.2025 № 140.</text:p>
<text:h text:outline-level="2">3. СРОК ДЕЙСТВИЯ И ПОРЯДОК ОТЗЫВА</text:h>
<text:p>3.1. Настоящее согласие действует со дня его подписания в течение всего срока действия трудового договора и в течение 5 (пяти) лет после его прекращения в соответствии со сроками архивного хранения кадровых документов.</text:p>
<text:p>3.2. Настоящее согласие может быть отозвано письменным заявлением, направленным в адрес Оператора заказным почтовым отправлением с описью вложения.</text:p>
<text:p>Субъект персональных данных: ____________________ / Морозова Е.С. /<text:line-break/>Дата подписания: 12 октября 2026 года</text:p>
`

	doc3TxtContent := `СОГЛАСИЕ НА ОБРАБОТКУ ПЕРСОНАЛЬНЫХ ДАННЫХ
(в соответствии с Федеральным законом № 152-ФЗ и Приказом Роскомнадзора от 19.06.2025 № 140)
г. Санкт-Петербург                                                                    12 октября 2026 года

Я, нижеподписавшаяся гражданка Российской Федерации Морозова Елена Сергеевна, дата рождения: 25.04.1994, документ, удостоверяющий личность: паспорт гражданина РФ серия 4014 номер 382910, выдан ТП № 15 ОУФМС России по Санкт-Петербургу и Ленинградской обл. в Приморском р-не г. Санкт-Петербурга 10.05.2014, код подразделения: 780-015; зарегистрированная по месту жительства по адресу: 197374, г. Санкт-Петербург, ул. Савушкина, д. 126, кв. 48; страховой номер индивидуального лицевого счета (СНИЛС): 200-111-222 45; идентификационный номер налогоплательщика (ИНН): 849949023870; контактный телефон: +7 (921) 987-65-43; адрес электронной почты: morozova.elena@datalab.spb.ru,

настоящим в соответствии со статьей 9 Федерального закона от 27.07.2006 № 152-ФЗ «О персональных данных» свободно, своей волей и в своем интересе даю безусловное согласие Обществу с ограниченной ответственностью «Аналитика Данных Северо-Запад» (ОГРН: 1187847123980, ИНН: 7814987650, адрес местонахождения: 197374, г. Санкт-Петербург, ул. Савушкина, д. 126, лит. Б, оф. 302) на обработку моих персональных данных со следующими условиями:

1. ПЕРЕЧЕНЬ ПЕРСОНАЛЬНЫХ ДАННЫХ
Обработке подлежат: фамилия, имя, отчество; число, месяц и год рождения; реквизиты паспорта РФ; адрес постоянной регистрации и фактического проживания; СНИЛС; ИНН; номера телефонов и адреса электронной почты; сведения о трудовой деятельности и квалификации; банковские реквизиты: номер карты МИР 2200 3979 6984 3733, расчетный счет 40817810300009876543 в Банк ВТБ (ПАО).

2. ЦЕЛИ ОБРАБОТКИ И ДЕПЕРСОНАЛИЗАЦИЯ ДАННЫХ
2.1. Персональные данные обрабатываются в целях заключения и исполнения трудового договора, исчисления налоговых платежей в ФНС РФ, начисления выплат Социального фонда России (СФР) и ведения кадрового делопроизводства.
2.2. Настоящим выражаю согласие на осуществление Оператором статистической и семантической деперсонализации (обезличивания) моих персональных данных в соответствии с методиками и требованиями Приказа Роскомнадзора от 19.06.2025 № 140.

3. СРОК ДЕЙСТВИЯ И ПОРЯДОК ОТЗЫВА
3.1. Настоящее согласие действует со дня его подписания в течение всего срока действия трудового договора и в течение 5 (пяти) лет после его прекращения в соответствии со сроками архивного хранения кадровых документов.
3.2. Настоящее согласие может быть отозвано письменным заявлением, направленным в адрес Оператора заказным почтовым отправлением с описью вложения.

Субъект персональных данных: ____________________ / Морозова Е.С. /
Дата подписания: 12 октября 2026 года
`

	doc3PdfPages := [][]string{
		{
			"СОГЛАСИЕ НА ОБРАБОТКУ ПЕРСОНАЛЬНЫХ ДАННЫХ (Страница 1 из 2)",
			"по 152-ФЗ РФ и Приказу Роскомнадзора от 19.06.2025 № 140",
			"г. Санкт-Петербург, дата подписания: 12 октября 2026 года",
			"Оператор: ООО Аналитика Данных Северо-Запад, ИНН 7814987650",
			"Субъект персональных данных: Морозова Елена Сергеевна",
			"Дата рождения: 25.04.1994",
			"Паспорт гражданина РФ: 4014 382910, код подразделения 780-015",
			"СНИЛС: 200-111-222 45, ИНН: 849949023870",
			"Адрес регистрации: 197374, г. Санкт-Петербург, ул. Савушкина, д. 126, кв. 48",
			"Телефон: +7 (921) 987-65-43, email: morozova.elena@datalab.spb.ru",
			"Банковская карта МИР: 2200 3979 6984 3733, счет 40817810300009876543",
		},
		{
			"СОГЛАСИЕ НА ОБРАБОТКУ ПЕРСОНАЛЬНЫХ ДАННЫХ (Страница 2 из 2)",
			"Цели обработки: трудовые отношения, выплата вознаграждения, отчетность в СФР и ФНС",
			"Подтверждаю право на деперсонализацию данных алгоритмами AES-256-GCM",
			"Срок действия согласия: на весь срок трудового договора + 5 лет",
			"Подпись субъекта персональных данных:",
			"Морозова Елена Сергеевна ____________________",
			"Дата: 12.10.2026 г.",
		},
	}

	createDocx(filepath.Join(dir3, "03_soglasie_pdn.docx"), doc3DocxBody)
	createOdt(filepath.Join(dir3, "03_soglasie_pdn.odt"), doc3OdtBody)
	createTxt(filepath.Join(dir3, "03_soglasie_pdn.txt"), doc3TxtContent)
	createMultiPagePdf(filepath.Join(dir3, "03_soglasie_pdn.pdf"), doc3PdfPages)

	// =========================================================================
	// ДОКУМЕНТ 4: ЗАЯВЛЕНИЕ НА ПОЛУЧЕНИЕ КОМПЕНСАЦИИ ЗА ИСПОЛЬЗОВАНИЕ ОБОРУДОВАНИЯ
	// =========================================================================
	fmt.Println("\n📄 [4/5] Создание: «Заявление на выплату компенсации за личное оборудование (ст. 312.6 ТК РФ)»...")

	doc4DocxBody := `
<w:p><w:pPr><w:jc w:val="right"/><w:spacing w:line="240" w:lineRule="auto"/></w:pPr><w:r><w:t>Генеральному директору&#10;ООО «Инновационные Системы»&#10;Сидорову А.В.&#10;от Ведущего инженера-разработчика&#10;Соколова Дмитрия Михайловича&#10;Паспорт: 4515 892341, код подр. 770-045&#10;СНИЛС: 112-233-445 95, ИНН: 151424725977&#10;Адрес: 119048, г. Москва, ул. Усачева, д. 29, кв. 114&#10;Тел: +7 (916) 234-56-78&#10;Email: sokolov.dmitry@techcorp.ru</w:t></w:r></w:p>
<w:p><w:pPr><w:jc w:val="center"/><w:spacing w:before="240" w:after="180"/></w:pPr><w:r><w:rPr><w:b/><w:sz w:val="28"/></w:rPr><w:t>ЗАЯВЛЕНИЕ</w:t></w:r></w:p>
<w:p><w:pPr><w:jc w:val="center"/><w:spacing w:after="240"/></w:pPr><w:r><w:rPr><w:i/><w:sz w:val="22"/></w:rPr><w:t>о выплате компенсации за использование личного оборудования и связи при дистанционной работе</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>В соответствии со статьями 188 и 312.6 Трудового кодекса Российской Федерации, а также пунктом 3.2 Трудового договора № ТД-2026/048 от 05.10.2026 прошу возместить расходы и выплатить ежемесячную компенсацию за использование в производственных целях принадлежащего мне личного компьютерного оборудования, программного обеспечения и каналов связи за октябрь 2026 года в общем размере 8 500 (восемь тысяч пятьсот) рублей 00 копеек.</w:t></w:r></w:p>
<w:p><w:pPr><w:spacing w:before="120" w:after="60"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>Расчет заявляемой суммы компенсации за октябрь 2026 г.:</w:t></w:r></w:p>
<w:tbl>
  <w:tblPr>
    <w:tblW w:w="9200" w:type="dxa"/>
    <w:tblBorders>
      <w:top w:val="single" w:sz="4" w:space="0" w:color="CCCCCC"/>
      <w:left w:val="single" w:sz="4" w:space="0" w:color="CCCCCC"/>
      <w:bottom w:val="single" w:sz="4" w:space="0" w:color="CCCCCC"/>
      <w:right w:val="single" w:sz="4" w:space="0" w:color="CCCCCC"/>
      <w:insideH w:val="single" w:sz="4" w:space="0" w:color="E0E0E0"/>
      <w:insideV w:val="single" w:sz="4" w:space="0" w:color="E0E0E0"/>
    </w:tblBorders>
  </w:tblPr>
  <w:tr>
    <w:trPr><w:tblHeader/></w:trPr>
    <w:tc><w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="F2F4F7"/></w:tcPr><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>№</w:t></w:r></w:p></w:tc>
    <w:tc><w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="F2F4F7"/></w:tcPr><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Статья расходов / Наименование оборудования</w:t></w:r></w:p></w:tc>
    <w:tc><w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="F2F4F7"/></w:tcPr><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Сумма к компенсации (руб.)</w:t></w:r></w:p></w:tc>
  </w:tr>
  <w:tr>
    <w:tc><w:p><w:r><w:t>1</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>Амортизация ноутбука Apple MacBook Pro 16" (рыночная стоимость 340 000 руб.)</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>4 500,00</w:t></w:r></w:p></w:tc>
  </w:tr>
  <w:tr>
    <w:tc><w:p><w:r><w:t>2</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>Компенсация расходов на электроэнергию при удаленной работе</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>1 200,00</w:t></w:r></w:p></w:tc>
  </w:tr>
  <w:tr>
    <w:tc><w:p><w:r><w:t>3</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>Услуги широкополосного доступа в сеть Интернет (провайдер ПАО «Ростелеком»)</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>1 500,00</w:t></w:r></w:p></w:tc>
  </w:tr>
  <w:tr>
    <w:tc><w:p><w:r><w:t>4</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>Служебная мобильная связь (оператор ПАО «МТС», номер +7 (916) 234-56-78)</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>1 300,00</w:t></w:r></w:p></w:tc>
  </w:tr>
  <w:tr>
    <w:tc><w:p><w:r><w:t></w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>ИТОГО к выплате:</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>8 500,00</w:t></w:r></w:p></w:tc>
  </w:tr>
</w:tbl>
<w:p><w:pPr><w:spacing w:before="180"/></w:pPr><w:r><w:t>Денежные средства прошу перечислить по следующим банковским реквизитам:&#10;Номер банковской карты МИР: 2200 8183 9572 5772&#10;Лицевой счет: 40817810500001234567 в ПАО Сбербанк&#10;БИК: 044525225, к/с: 30101810400000000225&#10;ИНН получателя: 151424725977, СНИЛС: 112-233-445 95</w:t></w:r></w:p>
<w:p><w:pPr><w:spacing w:before="180"/></w:pPr><w:r><w:t>Приложение: Копии электронных квитанций об оплате связи и интернета за октябрь 2026 г. на 2 л.</w:t></w:r></w:p>
<w:p><w:pPr><w:spacing w:before="180"/></w:pPr><w:r><w:t>14 октября 2026 года&#10;Подпись: __________________ / Соколов Д.М. /</w:t></w:r></w:p>
`

	doc4OdtBody := `
<text:p>Генеральному директору<text:line-break/>ООО «Инновационные Системы»<text:line-break/>Сидорову А.В.<text:line-break/>от Ведущего инженера-разработчика<text:line-break/>Соколова Дмитрия Михайловича<text:line-break/>Паспорт: 4515 892341, код подр. 770-045<text:line-break/>СНИЛС: 112-233-445 95, ИНН: 151424725977<text:line-break/>Адрес: 119048, г. Москва, ул. Усачева, д. 29, кв. 114<text:line-break/>Тел: +7 (916) 234-56-78<text:line-break/>Email: sokolov.dmitry@techcorp.ru</text:p>
<text:h text:outline-level="1">ЗАЯВЛЕНИЕ</text:h>
<text:p>о выплате компенсации за использование личного оборудования и связи при дистанционной работе</text:p>
<text:p>В соответствии со статьями 188 и 312.6 Трудового кодекса Российской Федерации, а также пунктом 3.2 Трудового договора № ТД-2026/048 от 05.10.2026 прошу возместить расходы и выплатить ежемесячную компенсацию за использование в производственных целях принадлежащего мне личного компьютерного оборудования, программного обеспечения и каналов связи за октябрь 2026 года в общем размере 8 500 (восемь тысяч пятьсот) рублей 00 копеек.</text:p>
<text:h text:outline-level="2">Расчет заявляемой суммы компенсации за октябрь 2026 г.:</text:h>
<table:table table:name="Расчет">
  <table:table-header-rows>
    <table:table-row>
      <table:table-cell><text:p>№</text:p></table:table-cell>
      <table:table-cell><text:p>Статья расходов / Наименование оборудования</text:p></table:table-cell>
      <table:table-cell><text:p>Сумма к компенсации (руб.)</text:p></table:table-cell>
    </table:table-row>
  </table:table-header-rows>
  <table:table-row>
    <table:table-cell><text:p>1</text:p></table:table-cell>
    <table:table-cell><text:p>Амортизация ноутбука Apple MacBook Pro 16" (рыночная стоимость 340 000 руб.)</text:p></table:table-cell>
    <table:table-cell><text:p>4 500,00</text:p></table:table-cell>
  </table:table-row>
  <table:table-row>
    <table:table-cell><text:p>2</text:p></table:table-cell>
    <table:table-cell><text:p>Компенсация расходов на электроэнергию при удаленной работе</text:p></table:table-cell>
    <table:table-cell><text:p>1 200,00</text:p></table:table-cell>
  </table:table-row>
  <table:table-row>
    <table:table-cell><text:p>3</text:p></table:table-cell>
    <table:table-cell><text:p>Услуги широкополосного доступа в сеть Интернет (провайдер ПАО «Ростелеком»)</text:p></table:table-cell>
    <table:table-cell><text:p>1 500,00</text:p></table:table-cell>
  </table:table-row>
  <table:table-row>
    <table:table-cell><text:p>4</text:p></table:table-cell>
    <table:table-cell><text:p>Служебная мобильная связь (оператор ПАО «МТС», номер +7 (916) 234-56-78)</text:p></table:table-cell>
    <table:table-cell><text:p>1 300,00</text:p></table:table-cell>
  </table:table-row>
  <table:table-row>
    <table:table-cell><text:p></text:p></table:table-cell>
    <table:table-cell><text:p>ИТОГО к выплате:</text:p></table:table-cell>
    <table:table-cell><text:p>8 500,00</text:p></table:table-cell>
  </table:table-row>
</table:table>
<text:p>Денежные средства прошу перечислить по следующим банковским реквизитам:<text:line-break/>Номер банковской карты МИР: 2200 8183 9572 5772<text:line-break/>Лицевой счет: 40817810500001234567 в ПАО Сбербанк<text:line-break/>БИК: 044525225, к/с: 30101810400000000225<text:line-break/>ИНН получателя: 151424725977, СНИЛС: 112-233-445 95</text:p>
<text:p>Приложение: Копии электронных квитанций об оплате связи и интернета за октябрь 2026 г. на 2 л.</text:p>
<text:p>14 октября 2026 года<text:line-break/>Подпись: __________________ / Соколов Д.М. /</text:p>
`

	doc4TxtContent := `Генеральному директору
ООО «Инновационные Системы»
Сидорову А.В.
от Ведущего инженера-разработчика
Соколова Дмитрия Михайловича
Паспорт: 4515 892341, код подр. 770-045
СНИЛС: 112-233-445 95, ИНН: 151424725977
Адрес: 119048, г. Москва, ул. Усачева, д. 29, кв. 114
Тел: +7 (916) 234-56-78
Email: sokolov.dmitry@techcorp.ru

ЗАЯВЛЕНИЕ
о выплате компенсации за использование личного оборудования и связи при дистанционной работе

В соответствии со статьями 188 и 312.6 Трудового кодекса Российской Федерации, а также пунктом 3.2 Трудового договора № ТД-2026/048 от 05.10.2026 прошу возместить расходы и выплатить ежемесячную компенсацию за использование в производственных целях принадлежащего мне личного компьютерного оборудования, программного обеспечения и каналов связи за октябрь 2026 года в общем размере 8 500 (восемь тысяч пятьсот) рублей 00 копеек.

Расчет заявляемой суммы компенсации за октябрь 2026 г.:
1. Амортизация ноутбука Apple MacBook Pro 16" (рыночная стоимость 340 000 руб.) - 4 500,00 руб.
2. Компенсация расходов на электроэнергию при удаленной работе - 1 200,00 руб.
3. Услуги широкополосного доступа в сеть Интернет (провайдер ПАО «Ростелеком») - 1 500,00 руб.
4. Служебная мобильная связь (оператор ПАО «МТС», номер +7 (916) 234-56-78) - 1 300,00 руб.
ИТОГО к выплате: 8 500,00 руб.

Денежные средства прошу перечислить по следующим банковским реквизитам:
Номер банковской карты МИР: 2200 8183 9572 5772
Лицевой счет: 40817810500001234567 в ПАО Сбербанк
БИК: 044525225, к/с: 30101810400000000225
ИНН получателя: 151424725977, СНИЛС: 112-233-445 95

Приложение: Копии электронных квитанций об оплате связи и интернета за октябрь 2026 г. на 2 л.

14 октября 2026 года
Подпись: __________________ / Соколов Д.М. /
`

	doc4PdfPages := [][]string{
		{
			"ЗАЯВЛЕНИЕ О ВЫПЛАТЕ КОМПЕНСАЦИИ РАСХОДОВ (ст. 312.6 ТК РФ)",
			"Директору ООО Инновационные Системы Сидорову А.В.",
			"от сотрудника: Соколов Дмитрий Михайлович",
			"Паспорт гражданина РФ: 4515 892341, код подразделения 770-045",
			"СНИЛС: 112-233-445 95, ИНН: 151424725977",
			"Адрес проживания: 119048, г. Москва, ул. Усачева, д. 29, корп. 2, кв. 114",
			"Телефон: +7 (916) 234-56-78, email: sokolov.dmitry@techcorp.ru",
			"Прошу выплатить компенсацию за использование личного оборудования за октябрь 2026 г.",
			"Сумма компенсации к выплате: 8 500 рублей 00 копеек",
			"Реквизиты перечисления: карта МИР 2200 8183 9572 5772, счет 40817810500001234567",
			"Дата заявления: 14 октября 2026 года, подпись: Соколов Д.М.",
		},
	}

	createDocx(filepath.Join(dir4, "04_zayavlenie_kompensaciya.docx"), doc4DocxBody)
	createOdt(filepath.Join(dir4, "04_zayavlenie_kompensaciya.odt"), doc4OdtBody)
	createTxt(filepath.Join(dir4, "04_zayavlenie_kompensaciya.txt"), doc4TxtContent)
	createMultiPagePdf(filepath.Join(dir4, "04_zayavlenie_kompensaciya.pdf"), doc4PdfPages)

	// =========================================================================
	// ДОКУМЕНТ 5: АКТ ПРИЕМА-ПЕРЕДАЧИ ОБОРУДОВАНИЯ И МАТЕРИАЛЬНЫХ ЦЕННОСТЕЙ (ТМЦ)
	// =========================================================================
	fmt.Println("\n📄 [5/5] Создание: «Акт приема-передачи оборудования и ТМЦ № АП-2026/088»...")

	doc5DocxBody := `
<w:p><w:pPr><w:jc w:val="right"/></w:pPr><w:r><w:t>Приложение № 2 к Трудовому договору&#10;№ ТД-2026/048 от 05.10.2026 г.</w:t></w:r></w:p>
<w:p><w:pPr><w:jc w:val="center"/><w:spacing w:before="180" w:after="120"/></w:pPr><w:r><w:rPr><w:b/><w:sz w:val="28"/></w:rPr><w:t>АКТ ПРИЕМА-ПЕРЕДАЧИ ОБОРУДОВАНИЯ И МАТЕРИАЛЬНЫХ ЦЕННОСТЕЙ № АП-2026/088</w:t></w:r></w:p>
<w:p><w:pPr><w:jc w:val="both"/><w:spacing w:after="180"/></w:pPr><w:r><w:t>г. Королев, Московская область                                                        15 октября 2026 года</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>Настоящий Акт составлен о том, что Начальник отдела IT-инфраструктуры Кузнецов Игорь Павлович передал, а Инженер технической поддержки Ковалев Артем Николаевич (Табельный номер: Таб. № 04821), именуемый в дальнейшем «Материально ответственное лицо» (МОЛ), принял во временное владение и пользование для служебных целей следующее имущество:</w:t></w:r></w:p>
<w:tbl>
  <w:tblPr>
    <w:tblW w:w="9200" w:type="dxa"/>
    <w:tblBorders>
      <w:top w:val="single" w:sz="4" w:space="0" w:color="CCCCCC"/>
      <w:left w:val="single" w:sz="4" w:space="0" w:color="CCCCCC"/>
      <w:bottom w:val="single" w:sz="4" w:space="0" w:color="CCCCCC"/>
      <w:right w:val="single" w:sz="4" w:space="0" w:color="CCCCCC"/>
      <w:insideH w:val="single" w:sz="4" w:space="0" w:color="E0E0E0"/>
      <w:insideV w:val="single" w:sz="4" w:space="0" w:color="E0E0E0"/>
    </w:tblBorders>
  </w:tblPr>
  <w:tr>
    <w:trPr><w:tblHeader/></w:trPr>
    <w:tc><w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="F2F4F7"/></w:tcPr><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>№</w:t></w:r></w:p></w:tc>
    <w:tc><w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="F2F4F7"/></w:tcPr><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Наименование ТМЦ</w:t></w:r></w:p></w:tc>
    <w:tc><w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="F2F4F7"/></w:tcPr><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Серийный номер (S/N)</w:t></w:r></w:p></w:tc>
    <w:tc><w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="F2F4F7"/></w:tcPr><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Инвентарный №</w:t></w:r></w:p></w:tc>
    <w:tc><w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="F2F4F7"/></w:tcPr><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Стоимость (руб.)</w:t></w:r></w:p></w:tc>
  </w:tr>
  <w:tr>
    <w:tc><w:p><w:r><w:t>1</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>Ноутбук Apple MacBook Pro 16" M3 Max</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>C02G4081MD6R</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>ИНВ-009412</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>340 000,00</w:t></w:r></w:p></w:tc>
  </w:tr>
  <w:tr>
    <w:tc><w:p><w:r><w:t>2</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>Монитор Dell UltraSharp 27" 4K U2723QE</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>CN-0P821K-74261</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>ИНВ-009413</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>68 000,00</w:t></w:r></w:p></w:tc>
  </w:tr>
  <w:tr>
    <w:tc><w:p><w:r><w:t>3</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>Токен аппаратный Рутокен ЭЦП 3.0 Type-C</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>3829104521</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>ИНВ-009414</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>6 500,00</w:t></w:r></w:p></w:tc>
  </w:tr>
</w:tbl>
<w:p><w:pPr><w:spacing w:before="180" w:after="60"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>Сведения о материально ответственном лице (МОЛ):</w:t></w:r></w:p>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/><w:spacing w:line="276" w:lineRule="auto"/></w:pPr><w:r><w:t>ФИО: Ковалев Артем Николаевич, табельный номер: Таб. № 04821&#10;Паспорт гражданина РФ: 5018 734912, выдан ГУ МВД России по Московской области 05.03.2018, код подразделения 500-023&#10;СНИЛС: 159-753-246 09, ИНН: 777695778193&#10;Адрес регистрации: 141070, Московская обл., г. Королев, пр-кт Космонавтов, д. 15, кв. 72&#10;Телефон: +7 (903) 555-12-34, email: kovalev.a@vector-it.ru&#10;Карта МИР для компенсаций: 2200 0760 3765 1548, расчетный счет 40817810700004561234 в АО Альфа-Банк</w:t></w:r></w:p>
<w:p><w:pPr><w:spacing w:before="180"/></w:pPr><w:r><w:t>Оборудование передано в исправном техническом состоянии. Претензий по комплектности стороны не имеют.&#10;&#10;Передал: __________________ / Кузнецов И.П. /&#10;Принял:   __________________ / Ковалев А.Н. /&#10;15 октября 2026 года</w:t></w:r></w:p>
`

	doc5OdtBody := `
<text:p>Приложение № 2 к Трудовому договору<text:line-break/>№ ТД-2026/048 от 05.10.2026 г.</text:p>
<text:h text:outline-level="1">АКТ ПРИЕМА-ПЕРЕДАЧИ ОБОРУДОВАНИЯ И МАТЕРИАЛЬНЫХ ЦЕННОСТЕЙ № АП-2026/088</text:h>
<text:p>г. Королев, Московская область                                                        15 октября 2026 года</text:p>
<text:p>Настоящий Акт составлен о том, что Начальник отдела IT-инфраструктуры Кузнецов Игорь Павлович передал, а Инженер технической поддержки Ковалев Артем Николаевич (Табельный номер: Таб. № 04821), именуемый в дальнейшем «Материально ответственное лицо» (МОЛ), принял во временное владение и пользование для служебных целей следующее имущество:</text:p>
<table:table table:name="Спецификация">
  <table:table-header-rows>
    <table:table-row>
      <table:table-cell><text:p>№</text:p></table:table-cell>
      <table:table-cell><text:p>Наименование ТМЦ</text:p></table:table-cell>
      <table:table-cell><text:p>Серийный номер (S/N)</text:p></table:table-cell>
      <table:table-cell><text:p>Инвентарный №</text:p></table:table-cell>
      <table:table-cell><text:p>Стоимость (руб.)</text:p></table:table-cell>
    </table:table-row>
  </table:table-header-rows>
  <table:table-row>
    <table:table-cell><text:p>1</text:p></table:table-cell>
    <table:table-cell><text:p>Ноутбук Apple MacBook Pro 16" M3 Max</text:p></table:table-cell>
    <table:table-cell><text:p>C02G4081MD6R</text:p></table:table-cell>
    <table:table-cell><text:p>ИНВ-009412</text:p></table:table-cell>
    <table:table-cell><text:p>340 000,00</text:p></table:table-cell>
  </table:table-row>
  <table:table-row>
    <table:table-cell><text:p>2</text:p></table:table-cell>
    <table:table-cell><text:p>Монитор Dell UltraSharp 27" 4K U2723QE</text:p></table:table-cell>
    <table:table-cell><text:p>CN-0P821K-74261</text:p></table:table-cell>
    <table:table-cell><text:p>ИНВ-009413</text:p></table:table-cell>
    <table:table-cell><text:p>68 000,00</text:p></table:table-cell>
  </table:table-row>
  <table:table-row>
    <table:table-cell><text:p>3</text:p></table:table-cell>
    <table:table-cell><text:p>Токен аппаратный Рутокен ЭЦП 3.0 Type-C</text:p></table:table-cell>
    <table:table-cell><text:p>3829104521</text:p></table:table-cell>
    <table:table-cell><text:p>ИНВ-009414</text:p></table:table-cell>
    <table:table-cell><text:p>6 500,00</text:p></table:table-cell>
  </table:table-row>
</table:table>
<text:h text:outline-level="2">Сведения о материально ответственном лице (МОЛ):</text:h>
<text:p>ФИО: Ковалев Артем Николаевич, табельный номер: Таб. № 04821<text:line-break/>Паспорт гражданина РФ: 5018 734912, выдан ГУ МВД России по Московской области 05.03.2018, код подразделения 500-023<text:line-break/>СНИЛС: 159-753-246 09, ИНН: 777695778193<text:line-break/>Адрес регистрации: 141070, Московская обл., г. Королев, пр-кт Космонавтов, д. 15, кв. 72<text:line-break/>Телефон: +7 (903) 555-12-34, email: kovalev.a@vector-it.ru<text:line-break/>Карта МИР для компенсаций: 2200 0760 3765 1548, расчетный счет 40817810700004561234 в АО Альфа-Банк</text:p>
<text:p>Оборудование передано в исправном техническом состоянии. Претензий по комплектности стороны не имеют.<text:line-break/><text:line-break/>Передал: __________________ / Кузнецов И.П. /<text:line-break/>Принял:   __________________ / Ковалев А.Н. /<text:line-break/>15 октября 2026 года</text:p>
`

	doc5TxtContent := `Приложение № 2 к Трудовому договору
№ ТД-2026/048 от 05.10.2026 г.

АКТ ПРИЕМА-ПЕРЕДАЧИ ОБОРУДОВАНИЯ И МАТЕРИАЛЬНЫХ ЦЕННОСТЕЙ № АП-2026/088
г. Королев, Московская область                                                        15 октября 2026 года

Настоящий Акт составлен о том, что Начальник отдела IT-инфраструктуры Кузнецов Игорь Павлович передал, а Инженер технической поддержки Ковалев Артем Николаевич (Табельный номер: Таб. № 04821), именуемый в дальнейшем «Материально ответственное лицо» (МОЛ), принял во временное владение и пользование для служебных целей следующее имущество:

Спецификация ТМЦ:
1. Ноутбук Apple MacBook Pro 16" M3 Max (S/N: C02G4081MD6R, ИНВ-009412) - 340 000,00 руб.
2. Монитор Dell UltraSharp 27" 4K U2723QE (S/N: CN-0P821K-74261, ИНВ-009413) - 68 000,00 руб.
3. Токен аппаратный Рутокен ЭЦП 3.0 Type-C (S/N: 3829104521, ИНВ-009414) - 6 500,00 руб.

Сведения о материально ответственном лице (МОЛ):
ФИО: Ковалев Артем Николаевич, табельный номер: Таб. № 04821
Паспорт гражданина РФ: 5018 734912, выдан ГУ МВД России по Московской области 05.03.2018, код подразделения 500-023
СНИЛС: 159-753-246 09, ИНН: 777695778193
Адрес регистрации: 141070, Московская обл., г. Королев, пр-кт Космонавтов, д. 15, кв. 72
Телефон: +7 (903) 555-12-34, email: kovalev.a@vector-it.ru
Карта МИР для компенсаций: 2200 0760 3765 1548, расчетный счет 40817810700004561234 в АО Альфа-Банк

Оборудование передано в исправном техническом состоянии. Претензий по комплектности стороны не имеют.

Передал: __________________ / Кузнецов И.П. /
Принял:   __________________ / Ковалев А.Н. /
15 октября 2026 года
`

	doc5PdfPages := [][]string{
		{
			"АКТ ПРИЕМА-ПЕРЕДАЧИ ОБОРУДОВАНИЯ № АП-2026/088 (Страница 1 из 2)",
			"г. Королев, Московская область, дата: 15 октября 2026 года",
			"Передал: Начальник отдела IT-инфраструктуры Кузнецов Игорь Павлович",
			"Принял: Инженер технической поддержки Ковалев Артем Николаевич, Таб. № 04821",
			"СПЕЦИФИКАЦИЯ ОБОРУДОВАНИЯ:",
			"1. Ноутбук Apple MacBook Pro 16 M3 Max, S/N C02G4081MD6R, ИНВ-009412",
			"2. Монитор Dell UltraSharp 27 4K, S/N CN-0P821K-74261, ИНВ-009413",
			"3. Токен Рутокен ЭЦП 3.0 Type-C, S/N 3829104521, ИНВ-009414",
			"Техническое состояние: полностью исправно, комплектность полная",
		},
		{
			"АКТ ПРИЕМА-ПЕРЕДАЧИ ОБОРУДОВАНИЯ № АП-2026/088 (Страница 2 из 2)",
			"СВЕДЕНИЯ О МАТЕРИАЛЬНО ОТВЕТСТВЕННОМ ЛИЦЕ:",
			"Сотрудник: Ковалев Артем Николаевич, Таб. № 04821",
			"Паспорт гражданина РФ: 5018 734912, выдан 05.03.2018, код подразделения 500-023",
			"СНИЛС: 159-753-246 09, ИНН: 777695778193",
			"Адрес регистрации: 141070, Московская обл., г. Королев, пр-кт Космонавтов, д. 15, кв. 72",
			"Контактный телефон: +7 (903) 555-12-34, email: kovalev.a@vector-it.ru",
			"Банковская карта МИР: 2200 0760 3765 1548, счет 40817810700004561234",
			"Подписи сторон: Кузнецов И.П. / Ковалев А.Н.",
			"Дата: 15 октября 2026 года",
		},
	}

	createDocx(filepath.Join(dir5, "05_akt_priema_peredachi_tmc.docx"), doc5DocxBody)
	createOdt(filepath.Join(dir5, "05_akt_priema_peredachi_tmc.odt"), doc5OdtBody)
	createTxt(filepath.Join(dir5, "05_akt_priema_peredachi_tmc.txt"), doc5TxtContent)
	createMultiPagePdf(filepath.Join(dir5, "05_akt_priema_peredachi_tmc.pdf"), doc5PdfPages)

	// =========================================================================
	// РАЗДЕЛ 6: ТАБЛИЧНЫЕ РЕЕСТРЫ 2026 ГОДА (XLSX, ODS, CSV)
	// =========================================================================
	fmt.Println("\n📊 [6/6] Создание: 3 табличных реестра 2026 года (XLSX, ODS, CSV)...")

	// Таблица 1: Реестр сотрудников 2026 года
	table1Headers := []string{
		"Таб. №",
		"ФИО",
		"Должность",
		"Дата рождения",
		"Паспорт (серия и номер)",
		"Код подразделения",
		"СНИЛС",
		"ИНН",
		"Телефон",
		"E-mail",
		"Адрес регистрации",
		"Карта МИР",
	}

	table1Rows := [][]string{
		{
			"04819",
			"Соколов Дмитрий Михайлович",
			"Ведущий разработчик",
			"14.08.1989",
			"4515 892341",
			"770-045",
			"112-233-445 95",
			"151424725977",
			"+7 (916) 234-56-78",
			"sokolov.dmitry@techcorp.ru",
			"119048, г. Москва, ул. Усачева, д. 29, корп. 2, кв. 114",
			"2200 8183 9572 5772",
		},
		{
			"04820",
			"Белов Максим Александрович",
			"Коммерческий директор",
			"18.06.1982",
			"4516 712398",
			"770-018",
			"182-287-392 95",
			"921793189287",
			"+7 (925) 444-55-66",
			"belov.m@globallogistics.ru",
			"125047, г. Москва, ул. 1-я Тверская-Ямская, д. 18, кв. 34",
			"2200 5882 8887 8927",
		},
		{
			"04821",
			"Морозова Елена Сергеевна",
			"Data Scientist",
			"25.04.1994",
			"4014 382910",
			"780-015",
			"200-111-222 45",
			"849949023870",
			"+7 (921) 987-65-43",
			"morozova.elena@datalab.spb.ru",
			"197374, г. Санкт-Петербург, ул. Савушкина, д. 126, кв. 48",
			"2200 3979 6984 3733",
		},
		{
			"04822",
			"Ковалев Артем Николаевич",
			"Инженер техподдержки",
			"12.11.1985",
			"5018 734912",
			"500-023",
			"159-753-246 09",
			"777695778193",
			"+7 (903) 555-12-34",
			"kovalev.a@vector-it.ru",
			"141070, Московская обл., г. Королев, пр-кт Космонавтов, д. 15, кв. 72",
			"2200 0760 3765 1548",
		},
	}

	createSpreadsheetXLSX(filepath.Join(dir6, "01_reestr_sotrudnikov_2026.xlsx"), "Сотрудники", table1Headers, table1Rows)
	createSpreadsheetODS(filepath.Join(dir6, "01_reestr_sotrudnikov_2026.ods"), "Сотрудники", table1Headers, table1Rows)
	createSpreadsheetCSV(filepath.Join(dir6, "01_reestr_sotrudnikov_2026.csv"), table1Headers, table1Rows)

	// Таблица 2: База клиентов 2026 года
	table2Headers := []string{
		"ID Клиента",
		"ФИО Клиента",
		"Телефон",
		"E-mail",
		"Паспорт РФ",
		"СНИЛС",
		"ИНН",
		"Город",
		"Адрес проживания",
		"Номер договора",
	}

	table2Rows := [][]string{
		{
			"CL-2026-001",
			"Соколов Дмитрий Михайлович",
			"+7 (916) 234-56-78",
			"sokolov.dmitry@techcorp.ru",
			"4515 892341",
			"112-233-445 95",
			"151424725977",
			"г. Москва",
			"119048, г. Москва, ул. Усачева, д. 29, корп. 2, кв. 114",
			"ДОГ-2026/101",
		},
		{
			"CL-2026-002",
			"Белов Максим Александрович",
			"+7 (925) 444-55-66",
			"belov.m@globallogistics.ru",
			"4516 712398",
			"182-287-392 95",
			"921793189287",
			"г. Москва",
			"125047, г. Москва, ул. 1-я Тверская-Ямская, д. 18, кв. 34",
			"ДОГ-2026/102",
		},
		{
			"CL-2026-003",
			"Морозова Елена Сергеевна",
			"+7 (921) 987-65-43",
			"morozova.elena@datalab.spb.ru",
			"4014 382910",
			"200-111-222 45",
			"849949023870",
			"г. Санкт-Петербург",
			"197374, г. Санкт-Петербург, ул. Савушкина, д. 126, кв. 48",
			"ДОГ-2026/103",
		},
		{
			"CL-2026-004",
			"Ковалев Артем Николаевич",
			"+7 (903) 555-12-34",
			"kovalev.a@vector-it.ru",
			"5018 734912",
			"159-753-246 09",
			"777695778193",
			"г. Королев",
			"141070, Московская обл., г. Королев, пр-кт Космонавтов, д. 15, кв. 72",
			"ДОГ-2026/104",
		},
	}

	createSpreadsheetXLSX(filepath.Join(dir6, "02_baza_klientov_2026.xlsx"), "Клиенты", table2Headers, table2Rows)
	createSpreadsheetODS(filepath.Join(dir6, "02_baza_klientov_2026.ods"), "Клиенты", table2Headers, table2Rows)
	createSpreadsheetCSV(filepath.Join(dir6, "02_baza_klientov_2026.csv"), table2Headers, table2Rows)

	// Таблица 3: Ведомость выплат заработной платы и компенсаций (октябрь 2026 года)
	table3Headers := []string{
		"№ п/п",
		"Табельный номер",
		"ФИО сотрудника",
		"Должность",
		"Сумма к выплате",
		"Номер карты МИР",
		"Расчетный счет",
		"СНИЛС",
		"ИНН",
		"Телефон",
	}

	table3Rows := [][]string{
		{
			"1",
			"Таб. № 04819",
			"Соколов Дмитрий Михайлович",
			"Ведущий разработчик",
			"328500.00",
			"2200 8183 9572 5772",
			"40817810500001234567",
			"112-233-445 95",
			"151424725977",
			"+7 (916) 234-56-78",
		},
		{
			"2",
			"Таб. № 04820",
			"Белов Максим Александрович",
			"Коммерческий директор",
			"450000.00",
			"2200 5882 8887 8927",
			"40817810800003412567",
			"182-287-392 95",
			"921793189287",
			"+7 (925) 444-55-66",
		},
		{
			"3",
			"Таб. № 04821",
			"Морозова Елена Сергеевна",
			"Data Scientist",
			"275000.00",
			"2200 3979 6984 3733",
			"40817810300009876543",
			"200-111-222 45",
			"849949023870",
			"+7 (921) 987-65-43",
		},
		{
			"4",
			"Таб. № 04822",
			"Ковалев Артем Николаевич",
			"Инженер техподдержки",
			"210000.00",
			"2200 0760 3765 1548",
			"40817810700004561234",
			"159-753-246 09",
			"777695778193",
			"+7 (903) 555-12-34",
		},
	}

	createSpreadsheetXLSX(filepath.Join(dir6, "03_vedomost_vyplat_10_2026.xlsx"), "Ведомость", table3Headers, table3Rows)
	createSpreadsheetODS(filepath.Join(dir6, "03_vedomost_vyplat_10_2026.ods"), "Ведомость", table3Headers, table3Rows)
	createSpreadsheetCSV(filepath.Join(dir6, "03_vedomost_vyplat_10_2026.csv"), table3Headers, table3Rows)

	// =========================================================================
	// ВЕРИФИКАЦИЯ ВСЕХ СОЗДАННЫХ ФАЙЛОВ ЧЕРЕЗ dep_service INSPECTOR
	// =========================================================================
	fmt.Println("\n=======================================================================")
	fmt.Println("🔍 Верификация всех созданных файлов через Inspector dep_service:")
	fmt.Println("=======================================================================")

	insp := inspector.New()
	ctx := context.Background()

	var allFiles []string
	_ = filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && !strings.HasPrefix(filepath.Base(path), ".") {
			allFiles = append(allFiles, path)
		}
		return nil
	})

	successCount := 0
	for _, fPath := range allFiles {
		relPath, _ := filepath.Rel(baseDir, fPath)
		fileInfo, err := insp.Inspect(ctx, fPath)
		if err != nil {
			fmt.Printf("❌ ОШИБКА [%s]: %v\n", relPath, err)
			continue
		}

		var entitySummary []string
		for _, e := range fileInfo.Entities {
			entitySummary = append(entitySummary, fmt.Sprintf("%s", e.ColumnName))
		}

		fmt.Printf("✅ [%-52s] Формат: %-5s | Стр/Строк: %2d | ПДн: %2d (%s)\n",
			relPath,
			fileInfo.Format,
			fileInfo.RowCount,
			len(fileInfo.Entities),
			strings.Join(entitySummary, ", "),
		)
		successCount++
	}

	fmt.Printf("\n✨ Успешно сгенерировано и верифицировано: %d из %d файлов!\n", successCount, len(allFiles))
}
