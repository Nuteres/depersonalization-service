package detector

// EntityType - тип распознанных персональных данных
type EntityType string

const (
	EntityFIO      EntityType = "FIO"
	EntitySNILS    EntityType = "SNILS"
	EntityINN      EntityType = "INN"
	EntityPassport EntityType = "PASSPORT"
	EntityPhone    EntityType = "PHONE"
	EntityEmail    EntityType = "EMAIL"
	EntityCard     EntityType = "BANK_CARD"
	EntityAddress  EntityType = "ADDRESS"
	EntityBirthday EntityType = "BIRTHDAY"
)

// MatchResult - результат распознавания ПДн
type MatchResult struct {
	Type       EntityType
	Value      string
	Confidence float64
}
