package model

import (
	"errors"
	"meth-enginev2/db"
	"time"
)

var (
	ErrMissingParent = errors.New("the post you're trying to reply to does not exist")
	ErrNotFound      = errors.New("not found")
)

type Post struct {
	ID        int64
	ParentID  *int64
	Message   string
	Sage      bool
	IPAddress string
	PosterID  string
	LastReply *int64
	CreatedAt time.Time
	UpdatedAt time.Time

	Tags []string
}

type User struct {
	ID           int64
	Username     string
	PasswordHash string
}

type Ban struct {
	ID        int64
	IPAddress string
	Reason    string
	ExpiresAt *time.Time
	CreatedBy string
	CreatedAt time.Time
}

type FilterAction string

const (
	FilterReplace FilterAction = "replace"
	FilterReject  FilterAction = "reject"
	FilterBan     FilterAction = "ban"
)

type Filter struct {
	ID          int64
	Regex       string
	Action      FilterAction
	Replacement string
	BanDays     int
	Note        string
	Hits        int64
	LastHitAt   *time.Time
	Broken      bool
}

type Store struct {
	db      *db.DB
	filters filterCache
}

type NewPost struct {
	ParentID *int64
	Message  string
	Sage     bool
	IP       string
	PosterID string

	MaxPosts int

	Tags []string
}
