package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"

	"dep_service/internal/version"
)

type IconDirEntry struct {
	Width       byte
	Height      byte
	ColorCount  byte
	Reserved    byte
	Planes      uint16
	BitCount    uint16
	BytesInRes  uint32
	ImageOffset uint32
}

type GrpIconDirEntry struct {
	Width      byte
	Height     byte
	ColorCount byte
	Reserved   byte
	Planes     uint16
	BitCount   uint16
	BytesInRes uint32
	ID         uint16
}

func main() {
	icoPath := "build/windows/icon.ico"
	outputPath := "cmd/app/icon_windows_amd64.syso"

	icoData, err := os.ReadFile(icoPath)
	if err != nil {
		fmt.Printf("Error reading %s: %v\n", icoPath, err)
		os.Exit(1)
	}

	r := bytes.NewReader(icoData)
	var reserved, imgType, count uint16
	binary.Read(r, binary.LittleEndian, &reserved)
	binary.Read(r, binary.LittleEndian, &imgType)
	binary.Read(r, binary.LittleEndian, &count)

	if imgType != 1 || count == 0 {
		fmt.Printf("Invalid ICO file: type=%d count=%d\n", imgType, count)
		os.Exit(1)
	}

	entries := make([]IconDirEntry, count)
	for i := 0; i < int(count); i++ {
		binary.Read(r, binary.LittleEndian, &entries[i])
	}

	images := make([][]byte, count)
	for i := 0; i < int(count); i++ {
		start := entries[i].ImageOffset
		end := start + entries[i].BytesInRes
		if int(end) > len(icoData) {
			fmt.Printf("ICO entry out of bounds\n")
			os.Exit(1)
		}
		images[i] = icoData[start:end]
	}

	// Build GRPICONDIR
	var grpBuf bytes.Buffer
	binary.Write(&grpBuf, binary.LittleEndian, uint16(0))
	binary.Write(&grpBuf, binary.LittleEndian, uint16(1))
	binary.Write(&grpBuf, binary.LittleEndian, count)
	for i := 0; i < int(count); i++ {
		grpEntry := GrpIconDirEntry{
			Width:      entries[i].Width,
			Height:     entries[i].Height,
			ColorCount: entries[i].ColorCount,
			Reserved:   entries[i].Reserved,
			Planes:     entries[i].Planes,
			BitCount:   entries[i].BitCount,
			BytesInRes: entries[i].BytesInRes,
			ID:         uint16(i + 1),
		}
		binary.Write(&grpBuf, binary.LittleEndian, grpEntry)
	}
	grpData := grpBuf.Bytes()

	manifestXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity version="%s.0" processorArchitecture="*" name="Depersonalizer" type="win32"/>
  <description>Деперсонализация ПД</description>
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3">
    <security>
      <requestedPrivileges>
        <requestedExecutionLevel level="asInvoker" uiAccess="false"/>
      </requestedPrivileges>
    </security>
  </trustInfo>
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1">
    <application>
      <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>
      <supportedOS Id="{1f676c76-80e1-4239-95bb-83d0f6d0da78}"/>
      <supportedOS Id="{4a2f28e3-53b9-4441-ba9c-d69d4a4a6e38}"/>
      <supportedOS Id="{35138b9a-5d96-4fbd-8e2d-a2440225f93a}"/>
    </application>
  </compatibility>
</assembly>`, version.Version)
	manifestData := []byte(manifestXML)

	type ResourceItem struct {
		TypeID int
		ResID  int
		Data   []byte
	}

	var items []ResourceItem
	for i := 0; i < int(count); i++ {
		items = append(items, ResourceItem{TypeID: 3, ResID: i + 1, Data: images[i]})
	}
	items = append(items, ResourceItem{TypeID: 14, ResID: 1, Data: grpData})
	items = append(items, ResourceItem{TypeID: 24, ResID: 1, Data: manifestData})

	var rsrcBuf bytes.Buffer

	typeDirsOffset := 16 + 3*8

	type3DirOffset := typeDirsOffset
	type3DirSize := 16 + int(count)*8

	type14DirOffset := type3DirOffset + type3DirSize
	type14DirSize := 16 + 1*8

	type24DirOffset := type14DirOffset + type14DirSize
	type24DirSize := 16 + 1*8

	langDirsOffset := type24DirOffset + type24DirSize
	totalItems := len(items)

	dataEntriesOffset := langDirsOffset + totalItems*24
	rawDataStartOffset := dataEntriesOffset + totalItems*16

	// Write Root Dir
	binary.Write(&rsrcBuf, binary.LittleEndian, uint32(0)) // Characteristics
	binary.Write(&rsrcBuf, binary.LittleEndian, uint32(0)) // TimeDateStamp
	binary.Write(&rsrcBuf, binary.LittleEndian, uint16(0)) // Major
	binary.Write(&rsrcBuf, binary.LittleEndian, uint16(0)) // Minor
	binary.Write(&rsrcBuf, binary.LittleEndian, uint16(0)) // NamedEntries
	binary.Write(&rsrcBuf, binary.LittleEndian, uint16(3)) // IdEntries (3, 14, 24)

	// Root Entries (must be sorted by Type ID: 3, 14, 24)
	binary.Write(&rsrcBuf, binary.LittleEndian, uint32(3))
	binary.Write(&rsrcBuf, binary.LittleEndian, uint32(0x80000000|type3DirOffset))

	binary.Write(&rsrcBuf, binary.LittleEndian, uint32(14))
	binary.Write(&rsrcBuf, binary.LittleEndian, uint32(0x80000000|type14DirOffset))

	binary.Write(&rsrcBuf, binary.LittleEndian, uint32(24))
	binary.Write(&rsrcBuf, binary.LittleEndian, uint32(0x80000000|type24DirOffset))

	// Type 3 Dir (RT_ICON)
	binary.Write(&rsrcBuf, binary.LittleEndian, uint32(0))
	binary.Write(&rsrcBuf, binary.LittleEndian, uint32(0))
	binary.Write(&rsrcBuf, binary.LittleEndian, uint16(0))
	binary.Write(&rsrcBuf, binary.LittleEndian, uint16(0))
	binary.Write(&rsrcBuf, binary.LittleEndian, uint16(0))
	binary.Write(&rsrcBuf, binary.LittleEndian, uint16(count))

	for i := 0; i < int(count); i++ {
		langOffset := langDirsOffset + i*24
		binary.Write(&rsrcBuf, binary.LittleEndian, uint32(i+1))
		binary.Write(&rsrcBuf, binary.LittleEndian, uint32(0x80000000|langOffset))
	}

	// Type 14 Dir (RT_GROUP_ICON)
	binary.Write(&rsrcBuf, binary.LittleEndian, uint32(0))
	binary.Write(&rsrcBuf, binary.LittleEndian, uint32(0))
	binary.Write(&rsrcBuf, binary.LittleEndian, uint16(0))
	binary.Write(&rsrcBuf, binary.LittleEndian, uint16(0))
	binary.Write(&rsrcBuf, binary.LittleEndian, uint16(0))
	binary.Write(&rsrcBuf, binary.LittleEndian, uint16(1))

	langOffset14 := langDirsOffset + int(count)*24
	binary.Write(&rsrcBuf, binary.LittleEndian, uint32(1)) // ID 1
	binary.Write(&rsrcBuf, binary.LittleEndian, uint32(0x80000000|langOffset14))

	// Type 24 Dir (RT_MANIFEST)
	binary.Write(&rsrcBuf, binary.LittleEndian, uint32(0))
	binary.Write(&rsrcBuf, binary.LittleEndian, uint32(0))
	binary.Write(&rsrcBuf, binary.LittleEndian, uint16(0))
	binary.Write(&rsrcBuf, binary.LittleEndian, uint16(0))
	binary.Write(&rsrcBuf, binary.LittleEndian, uint16(0))
	binary.Write(&rsrcBuf, binary.LittleEndian, uint16(1))

	langOffset24 := langDirsOffset + (int(count)+1)*24
	binary.Write(&rsrcBuf, binary.LittleEndian, uint32(1)) // ID 1
	binary.Write(&rsrcBuf, binary.LittleEndian, uint32(0x80000000|langOffset24))

	// Lang Dirs
	for i := 0; i < totalItems; i++ {
		binary.Write(&rsrcBuf, binary.LittleEndian, uint32(0))
		binary.Write(&rsrcBuf, binary.LittleEndian, uint32(0))
		binary.Write(&rsrcBuf, binary.LittleEndian, uint16(0))
		binary.Write(&rsrcBuf, binary.LittleEndian, uint16(0))
		binary.Write(&rsrcBuf, binary.LittleEndian, uint16(0))
		binary.Write(&rsrcBuf, binary.LittleEndian, uint16(1))

		dataEntryOff := dataEntriesOffset + i*16
		binary.Write(&rsrcBuf, binary.LittleEndian, uint32(0)) // Lang 0 (Neutral)
		binary.Write(&rsrcBuf, binary.LittleEndian, uint32(dataEntryOff))
	}

	// Calculate raw data offsets
	rawDataOffsets := make([]int, totalItems)
	currOff := rawDataStartOffset
	for i := 0; i < totalItems; i++ {
		if currOff%4 != 0 {
			currOff += (4 - (currOff % 4))
		}
		rawDataOffsets[i] = currOff
		currOff += len(items[i].Data)
	}

	// Write Data Entries
	var relocPositions []uint32
	for i := 0; i < totalItems; i++ {
		relocPositions = append(relocPositions, uint32(rsrcBuf.Len()))
		binary.Write(&rsrcBuf, binary.LittleEndian, uint32(rawDataOffsets[i])) // OffsetToData
		binary.Write(&rsrcBuf, binary.LittleEndian, uint32(len(items[i].Data))) // Size
		binary.Write(&rsrcBuf, binary.LittleEndian, uint32(0))                // CodePage
		binary.Write(&rsrcBuf, binary.LittleEndian, uint32(0))                // Reserved
	}

	// Write Raw Data
	for i := 0; i < totalItems; i++ {
		for rsrcBuf.Len() < rawDataOffsets[i] {
			rsrcBuf.WriteByte(0)
		}
		rsrcBuf.Write(items[i].Data)
	}

	// Align entire .rsrc section to 4 bytes
	for rsrcBuf.Len()%4 != 0 {
		rsrcBuf.WriteByte(0)
	}
	rsrcData := rsrcBuf.Bytes()

	// Build COFF Object File (.syso) for AMD64
	var coffBuf bytes.Buffer

	numRelocs := uint16(len(relocPositions))
	headerSize := 20 + 40
	ptrToRawData := uint32(headerSize)
	ptrToRelocs := ptrToRawData + uint32(len(rsrcData))
	ptrToSymbols := ptrToRelocs + uint32(numRelocs)*10

	// COFF File Header (20 bytes)
	binary.Write(&coffBuf, binary.LittleEndian, uint16(0x8664))      // Machine = IMAGE_FILE_MACHINE_AMD64
	binary.Write(&coffBuf, binary.LittleEndian, uint16(1))           // NumberOfSections = 1
	binary.Write(&coffBuf, binary.LittleEndian, uint32(0))           // TimeDateStamp
	binary.Write(&coffBuf, binary.LittleEndian, ptrToSymbols)        // PointerToSymbolTable
	binary.Write(&coffBuf, binary.LittleEndian, uint32(1))           // NumberOfSymbols = 1
	binary.Write(&coffBuf, binary.LittleEndian, uint16(0))           // SizeOfOptionalHeader = 0
	binary.Write(&coffBuf, binary.LittleEndian, uint16(0x0000))      // Characteristics

	// Section Header (40 bytes)
	var secName [8]byte
	copy(secName[:], ".rsrc")
	coffBuf.Write(secName[:])
	binary.Write(&coffBuf, binary.LittleEndian, uint32(0))                 // VirtualSize
	binary.Write(&coffBuf, binary.LittleEndian, uint32(0))                 // VirtualAddress
	binary.Write(&coffBuf, binary.LittleEndian, uint32(len(rsrcData)))     // SizeOfRawData
	binary.Write(&coffBuf, binary.LittleEndian, ptrToRawData)              // PointerToRawData
	binary.Write(&coffBuf, binary.LittleEndian, ptrToRelocs)               // PointerToRelocations
	binary.Write(&coffBuf, binary.LittleEndian, uint32(0))                 // PointerToLinenumbers
	binary.Write(&coffBuf, binary.LittleEndian, numRelocs)                 // NumberOfRelocations
	binary.Write(&coffBuf, binary.LittleEndian, uint16(0))                 // NumberOfLinenumbers
	binary.Write(&coffBuf, binary.LittleEndian, uint32(0x40000040))         // Characteristics

	// Write .rsrc data
	coffBuf.Write(rsrcData)

	// Write Relocations (10 bytes each)
	for _, pos := range relocPositions {
		binary.Write(&coffBuf, binary.LittleEndian, pos)             // VirtualAddress
		binary.Write(&coffBuf, binary.LittleEndian, uint32(0))        // SymbolTableIndex = 0 (.rsrc)
		binary.Write(&coffBuf, binary.LittleEndian, uint16(3))        // Type = IMAGE_REL_AMD64_ADDR32NB
	}

	// Write Symbol Table (18 bytes for symbol 0)
	var symName [8]byte
	copy(symName[:], ".rsrc")
	coffBuf.Write(symName[:])
	binary.Write(&coffBuf, binary.LittleEndian, uint32(0)) // Value
	binary.Write(&coffBuf, binary.LittleEndian, int16(1))  // SectionNumber = 1
	binary.Write(&coffBuf, binary.LittleEndian, uint16(0)) // Type
	binary.Write(&coffBuf, binary.LittleEndian, uint8(3))  // StorageClass = IMAGE_SYM_CLASS_STATIC
	binary.Write(&coffBuf, binary.LittleEndian, uint8(0))  // NumberOfAuxSymbols = 0

	// Write String Table Length (4 bytes)
	binary.Write(&coffBuf, binary.LittleEndian, uint32(4))

	err = os.WriteFile(outputPath, coffBuf.Bytes(), 0644)
	if err != nil {
		fmt.Printf("Error writing %s: %v\n", outputPath, err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated %s (%d bytes)\n", outputPath, coffBuf.Len())
}
