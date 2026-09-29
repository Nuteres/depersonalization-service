package anonymizer

import (
	"fmt"
	"strings"
)

type Gender int

const (
	GenderMale Gender = iota
	GenderFemale
)

func DetectGender(fio string) Gender {
	parts := strings.Fields(fio)
	for _, p := range parts {
		lower := strings.ToLower(p)
		if strings.HasSuffix(lower, "овна") || strings.HasSuffix(lower, "евна") || strings.HasSuffix(lower, "ична") || strings.HasSuffix(lower, "ая") || strings.HasSuffix(lower, "ова") || strings.HasSuffix(lower, "ева") {
			return GenderFemale
		}
	}
	return GenderMale
}

var MaleSurnames = []string{"Смирнов", "Иванов", "Кузнецов", "Попов", "Соколов", "Лебедев", "Козлов", "Новиков", "Морозов", "Петров", "Волков", "Соловьев", "Васильев", "Зайцев", "Павлов"}
var FemaleSurnames = []string{"Смирнова", "Иванова", "Кузнецова", "Попова", "Соколова", "Лебедева", "Козлова", "Новикова", "Морозова", "Петрова", "Волкова", "Соловьева", "Васильева", "Зайцева", "Павлова"}

var MaleNames = []string{"Александр", "Михаил", "Максим", "Артем", "Даниил", "Иван", "Дмитрий", "Кирилл", "Андрей", "Егор", "Роман", "Сергей", "Никита"}
var FemaleNames = []string{"София", "Анна", "Мария", "Алиса", "Ева", "Виктория", "Полина", "Екатерина", "Ксения", "Дарья", "Анастасия", "Ольга"}

var MalePatronymics = []string{"Александрович", "Михайлович", "Максимович", "Иванович", "Дмитриевич", "Андреевич", "Сергеевич", "Романович", "Павлович"}
var FemalePatronymics = []string{"Александровна", "Михайловна", "Максимовна", "Ивановна", "Дмитриевна", "Андреевна", "Сергеевна", "Романовна", "Павловна"}

func (a *Anonymizer) GenerateFIO(original string) string {
	seed := a.DeriveSeed(original)
	gender := DetectGender(original)

	if gender == GenderFemale {
		sIdx := a.RandInt(seed, 0, len(FemaleSurnames)-1)
		nIdx := a.RandInt(seed+1, 0, len(FemaleNames)-1)
		pIdx := a.RandInt(seed+2, 0, len(FemalePatronymics)-1)
		return fmt.Sprintf("%s %s %s", FemaleSurnames[sIdx], FemaleNames[nIdx], FemalePatronymics[pIdx])
	}
	sIdx := a.RandInt(seed, 0, len(MaleSurnames)-1)
	nIdx := a.RandInt(seed+1, 0, len(MaleNames)-1)
	pIdx := a.RandInt(seed+2, 0, len(MalePatronymics)-1)
	return fmt.Sprintf("%s %s %s", MaleSurnames[sIdx], MaleNames[nIdx], MalePatronymics[pIdx])
}
