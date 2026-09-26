package rbac

import "errors"

var (
	ErrTokenNotFound = errors.New("token not found")
	ErrTokenExpired  = errors.New("token expired")
	ErrUnauthorized  = errors.New("unauthorized")
)
