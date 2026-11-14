package model

import "errors"

// Common errors
var (
	ErrDatabase = errors.New("database error")
)

// User auth errors
var (
	ErrInvalidCredentials    = errors.New("invalid credentials")
	ErrUserEmptyCredentials  = errors.New("empty credentials")
	ErrEmailAlreadyTaken     = errors.New("email already taken")
	ErrEmailNotFound         = errors.New("email not found")
	ErrEmailAmbiguous        = errors.New("email matches multiple users")
	ErrUsernameAlreadyTaken  = errors.New("username already taken")
	ErrUsernameCaseCollision = errors.New("case-variant usernames must be resolved before the unique index")
)

// Team membership errors
var (
	ErrActiveTeamMembershipCollision = errors.New("duplicate active team memberships must be resolved before the unique index")
)

// Voucher redeem-order uniqueness
var (
	ErrAdtyVoucherTradeNoCollision = errors.New("duplicate voucher trade numbers must be resolved before the unique index")
)

// Token auth errors
var (
	ErrTokenNotProvided = errors.New("token not provided")
	ErrTokenInvalid     = errors.New("token invalid")
)

// 2FA errors
var ErrTwoFANotEnabled = errors.New("2fa not enabled")
var ErrTwoFAAlreadyEnabled = errors.New("2fa already enabled")
