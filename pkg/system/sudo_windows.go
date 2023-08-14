//go:build windows

package system

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func IsElevated() (bool, error) {
	var sid *windows.SID

	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid)

	if err != nil {
		return false, fmt.Errorf("sid error: %s", err)
	}

	defer windows.FreeSid(sid)

	token := windows.Token(0)

	member, err := token.IsMember(sid)

	if err != nil {
		return false, fmt.Errorf("token membership error: %s", err)
	}

	return member, nil
}
