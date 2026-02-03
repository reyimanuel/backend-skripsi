package user

type User struct {
	ID       int    `gorm:"column:id;primaryKey;autoIncrement;not null;<-create"`
	Name     string `gorm:"column:name;not null"`
	Email    string `gorm:"column:email;uniqueIndex;not null"`
	Password string `gorm:"column:password;not null"`
	Role     string `gorm:"column:role;type:varchar(10);not null"` // admin, superAdmin
}
