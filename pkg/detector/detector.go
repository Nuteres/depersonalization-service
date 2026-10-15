package detector

// EntityType - тип распознанных персональных данных
type EntityType string

const (
	EntityFIO        EntityType = "FIO"
	EntityLastName   EntityType = "LAST_NAME"
	EntityFirstName  EntityType = "FIRST_NAME"
	EntityPatronymic EntityType = "PATRONYMIC"
	EntitySNILS      EntityType = "SNILS"
	EntityINN        EntityType = "INN"
	EntityPassport   EntityType = "PASSPORT"
	EntityPhone      EntityType = "PHONE"
	EntityEmail      EntityType = "EMAIL"
	EntityCard       EntityType = "BANK_CARD"
	EntityAddress    EntityType = "ADDRESS"
	EntityBirthday   EntityType = "BIRTHDAY"
	EntityDate       EntityType = "DATE"
	EntityGrade      EntityType = "GRADE"
	EntityEmployeeID EntityType = "EMPLOYEE_ID"
)
