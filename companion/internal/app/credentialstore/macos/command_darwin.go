//go:build darwin && cgo

// Package macos provides the non-interactive native Keychain operations used
// by Operator's credential store.
package macos

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>

static OSStatus operator_keychain_put(
    const char *service, UInt32 service_length,
    const char *account, UInt32 account_length,
    const void *secret, UInt32 secret_length) {
  SecKeychainItemRef item = NULL;
  OSStatus status = SecKeychainFindGenericPassword(
      NULL, service_length, service, account_length, account,
      NULL, NULL, &item);
  if (status == errSecSuccess) {
    status = SecKeychainItemModifyContent(item, NULL, secret_length, secret);
    CFRelease(item);
    return status;
  }
  if (status != errSecItemNotFound) {
    return status;
  }
  return SecKeychainAddGenericPassword(
      NULL, service_length, service, account_length, account,
      secret_length, secret, NULL);
}

static OSStatus operator_keychain_get(
    const char *service, UInt32 service_length,
    const char *account, UInt32 account_length,
    UInt32 *secret_length, void **secret) {
  return SecKeychainFindGenericPassword(
      NULL, service_length, service, account_length, account,
      secret_length, secret, NULL);
}

static OSStatus operator_keychain_delete(
    const char *service, UInt32 service_length,
    const char *account, UInt32 account_length) {
  SecKeychainItemRef item = NULL;
  OSStatus status = SecKeychainFindGenericPassword(
      NULL, service_length, service, account_length, account,
      NULL, NULL, &item);
  if (status != errSecSuccess) {
    return status;
  }
  status = SecKeychainItemDelete(item);
  CFRelease(item);
  return status;
}
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"unsafe"
)

// Command implements the narrow security-command contract expected by the
// parent package without starting a subprocess or opening a terminal prompt.
type Command struct{}

func (Command) Run(ctx context.Context, input string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, errors.New("native keychain: operation is required")
	}
	service, account, err := identifiers(args)
	if err != nil {
		return nil, err
	}

	svc := C.CString(service)
	defer C.free(unsafe.Pointer(svc))
	acct := C.CString(account)
	defer C.free(unsafe.Pointer(acct))

	switch args[0] {
	case "add-generic-password":
		if input == "" {
			return nil, errors.New("native keychain: secret is required")
		}
		secret := []byte(input)
		status := C.operator_keychain_put(
			svc, C.UInt32(len(service)), acct, C.UInt32(len(account)),
			unsafe.Pointer(&secret[0]), C.UInt32(len(secret)),
		)
		return nil, statusError("store", status)
	case "find-generic-password":
		var secretLength C.UInt32
		var secret unsafe.Pointer
		status := C.operator_keychain_get(
			svc, C.UInt32(len(service)), acct, C.UInt32(len(account)),
			&secretLength, &secret,
		)
		if err := statusError("load", status); err != nil {
			return nil, err
		}
		defer C.SecKeychainItemFreeContent(nil, secret)
		return C.GoBytes(secret, C.int(secretLength)), nil
	case "delete-generic-password":
		status := C.operator_keychain_delete(
			svc, C.UInt32(len(service)), acct, C.UInt32(len(account)),
		)
		return nil, statusError("delete", status)
	default:
		return nil, fmt.Errorf("native keychain: unsupported operation %q", args[0])
	}
}

func identifiers(args []string) (string, string, error) {
	value := func(flag string) string {
		for i := 1; i+1 < len(args); i++ {
			if args[i] == flag {
				return args[i+1]
			}
		}
		return ""
	}
	service, account := value("-s"), value("-a")
	if service == "" || account == "" {
		return "", "", errors.New("native keychain: service and account are required")
	}
	return service, account, nil
}

func statusError(operation string, status C.OSStatus) error {
	if status == C.errSecSuccess {
		return nil
	}
	if status == C.errSecItemNotFound {
		return fmt.Errorf("native keychain %s: the specified item could not be found", operation)
	}
	return fmt.Errorf("native keychain %s failed with status %d", operation, int32(status))
}
