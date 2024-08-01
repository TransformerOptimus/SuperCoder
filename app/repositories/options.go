package repositories

import "gorm.io/gorm"

type RepositoryOption interface {
	applyRepositoryOption(*RepositoryOptions)
}

type RepositoryOptions struct {
	db    gorm.DB
	page  int
	limit int
}

type DBOption gorm.DB

func (o DBOption) applyRepositoryOption(ro *RepositoryOptions) {
	ro.db = gorm.DB(o)
}

type PageOption int

func (o PageOption) applyRepositoryOption(ro *RepositoryOptions) {
	ro.page = int(o)
}

type LimitOption int

func (o LimitOption) applyRepositoryOption(ro *RepositoryOptions) {
	ro.limit = int(o)
}

func WithDB(db gorm.DB) DBOption {
	return DBOption(db)
}

func WithPage(page int) PageOption {
	return PageOption(page)
}

func WithLimit(limit int) LimitOption {
	return LimitOption(limit)
}
