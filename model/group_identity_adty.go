package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// ErrNoPublicIdentityGroup means a new user has no explicit group and the
// historical default is not a public billing group. Insert must not fall
// through to the column default.
var ErrNoPublicIdentityGroup = errors.New("no public billing group is available for new users")

// IdentityGroupChangeAllowed reports whether an identity write may change the
// group. The current value stays valid so an existing hidden identity can still
// be edited. A new value must be a public billing group, never auto.
func IdentityGroupChangeAllowed(current, next string) bool {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if next == current {
		return true
	}
	if next == "" || next == "auto" || !ratio_setting.ContainsGroupRatio(next) {
		return false
	}
	_, public := setting.GetUserUsableGroupsCopy()[next]
	return public
}

// assignPublicIdentityForInsert stops an empty group from using the column
// default after that code has been removed from the public billing set.
// An explicit group is left unchanged; HTTP signup does not send one.
func assignPublicIdentityForInsert(user *User) error {
	if user == nil {
		return ErrNoPublicIdentityGroup
	}
	group := strings.TrimSpace(user.Group)
	if group != "" && group != "auto" {
		user.Group = group
		return nil
	}
	if !IdentityGroupChangeAllowed("", "default") {
		return ErrNoPublicIdentityGroup
	}
	user.Group = "default"
	return nil
}
