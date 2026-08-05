//go:build darwin && cgo

// Package macos provides the non-interactive native Keychain operations used
// by Operator's credential store.
package macos

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>

static OSStatus operator_keychain_disable_interaction(void) {
  return SecKeychainSetUserInteractionAllowed(0);
}

// Allow any local app to decrypt without a prompt. SecKeychainAddGenericPassword
// binds decrypt ACL to the caller's code signature (cdhash); go run / rebuilds
// then fail Get with -25293 while interaction is disabled. Pattern from Apple
// Keychain ACL docs + SecACLCreateWithSimpleContents(NULL apps).
static OSStatus operator_access_allow_any_app(CFStringRef descriptor, SecAccessRef *out) {
  SecAccessRef access = NULL;
  CFArrayRef none = CFArrayCreate(kCFAllocatorDefault, NULL, 0, &kCFTypeArrayCallBacks);
  if (none == NULL) {
    return errSecAllocate;
  }
  OSStatus status = SecAccessCreate(descriptor, none, &access);
  CFRelease(none);
  if (status != errSecSuccess) {
    return status;
  }

  CFArrayRef decryptACLs = SecAccessCopyMatchingACLList(access, kSecACLAuthorizationDecrypt);
  if (decryptACLs == NULL || CFArrayGetCount(decryptACLs) != 1) {
    if (decryptACLs != NULL) {
      CFRelease(decryptACLs);
    }
    CFRelease(access);
    return errSecInvalidACL;
  }
  SecACLRef oldACL = (SecACLRef)CFArrayGetValueAtIndex(decryptACLs, 0);
  CFArrayRef authorizations = SecACLCopyAuthorizations(oldACL);
  status = SecACLRemove(oldACL);
  CFRelease(decryptACLs);
  if (status != errSecSuccess) {
    if (authorizations != NULL) {
      CFRelease(authorizations);
    }
    CFRelease(access);
    return status;
  }

  SecACLRef newACL = NULL;
  status = SecACLCreateWithSimpleContents(access, NULL, descriptor, 0, &newACL);
  if (status != errSecSuccess) {
    if (authorizations != NULL) {
      CFRelease(authorizations);
    }
    CFRelease(access);
    return status;
  }
  if (authorizations != NULL) {
    status = SecACLUpdateAuthorizations(newACL, authorizations);
    CFRelease(authorizations);
    if (status != errSecSuccess) {
      CFRelease(access);
      return status;
    }
  }
  *out = access;
  return errSecSuccess;
}

static OSStatus operator_keychain_delete_query(const char *service, const char *account) {
  CFStringRef svc = CFStringCreateWithCString(NULL, service, kCFStringEncodingUTF8);
  CFStringRef acct = CFStringCreateWithCString(NULL, account, kCFStringEncodingUTF8);
  if (svc == NULL || acct == NULL) {
    if (svc != NULL) CFRelease(svc);
    if (acct != NULL) CFRelease(acct);
    return errSecAllocate;
  }
  const void *keys[] = { kSecClass, kSecAttrService, kSecAttrAccount };
  const void *vals[] = { kSecClassGenericPassword, svc, acct };
  CFDictionaryRef query = CFDictionaryCreate(
      NULL, keys, vals, 3,
      &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
  OSStatus status = SecItemDelete(query);
  CFRelease(query);
  CFRelease(svc);
  CFRelease(acct);
  if (status == errSecItemNotFound) {
    return errSecSuccess;
  }
  return status;
}

// Replace-on-update with any-app decrypt ACL so Get works across go run /
// rebuilt binaries without Keychain Allow prompts.
static OSStatus operator_keychain_put(
    const char *service, UInt32 service_length,
    const char *account, UInt32 account_length,
    const void *secret, UInt32 secret_length) {
  SecKeychainItemRef item = NULL;
  OSStatus status = SecKeychainFindGenericPassword(
      NULL, service_length, service, account_length, account,
      NULL, NULL, &item);
  if (status == errSecSuccess) {
    status = SecKeychainItemDelete(item);
    CFRelease(item);
    if (status != errSecSuccess) {
      return status;
    }
  } else if (status != errSecItemNotFound) {
    // Unreadable ACL (-25293): drop by query so we can recreate.
    status = operator_keychain_delete_query(service, account);
    if (status != errSecSuccess) {
      return status;
    }
  }

  CFStringRef desc = CFStringCreateWithCString(NULL, "Operator credentials", kCFStringEncodingUTF8);
  CFStringRef svc = CFStringCreateWithCString(NULL, service, kCFStringEncodingUTF8);
  CFStringRef acct = CFStringCreateWithCString(NULL, account, kCFStringEncodingUTF8);
  CFDataRef data = CFDataCreate(NULL, (const UInt8 *)secret, (CFIndex)secret_length);
  if (desc == NULL || svc == NULL || acct == NULL || data == NULL) {
    if (desc != NULL) CFRelease(desc);
    if (svc != NULL) CFRelease(svc);
    if (acct != NULL) CFRelease(acct);
    if (data != NULL) CFRelease(data);
    return errSecAllocate;
  }
  SecAccessRef access = NULL;
  status = operator_access_allow_any_app(desc, &access);
  CFRelease(desc);
  if (status != errSecSuccess) {
    CFRelease(svc);
    CFRelease(acct);
    CFRelease(data);
    return status;
  }

  // SecItemAdd + kSecAttrAccess is the documented way to attach a custom ACL
  // at create time (SecKeychainAddGenericPassword always binds the caller).
  CFMutableDictionaryRef attrs = CFDictionaryCreateMutable(
      NULL, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
  if (attrs == NULL) {
    CFRelease(access);
    CFRelease(svc);
    CFRelease(acct);
    CFRelease(data);
    return errSecAllocate;
  }
  CFDictionarySetValue(attrs, kSecClass, kSecClassGenericPassword);
  CFDictionarySetValue(attrs, kSecAttrService, svc);
  CFDictionarySetValue(attrs, kSecAttrAccount, acct);
  CFDictionarySetValue(attrs, kSecValueData, data);
  CFDictionarySetValue(attrs, kSecAttrAccess, access);
  status = SecItemAdd(attrs, NULL);
  CFRelease(attrs);
  CFRelease(access);
  CFRelease(svc);
  CFRelease(acct);
  CFRelease(data);
  return status;
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

var ErrInteractionNotAllowed = errors.New("native keychain interaction is disabled")

// ErrAuthFailed is macOS errSecAuthFailed (-25293): the item exists but this
// process cannot decrypt it under the current ACL.
var ErrAuthFailed = errors.New("native keychain auth failed (ACL rejects this binary)")

var statusInteractionNotAllowed = int32(C.errSecInteractionNotAllowed)
var statusAuthFailed = int32(C.errSecAuthFailed)

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
	if err := statusError("disable interaction", int32(C.operator_keychain_disable_interaction())); err != nil {
		return nil, err
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
		return nil, statusError("store", int32(status))
	case "find-generic-password":
		var secretLength C.UInt32
		var secret unsafe.Pointer
		status := C.operator_keychain_get(
			svc, C.UInt32(len(service)), acct, C.UInt32(len(account)),
			&secretLength, &secret,
		)
		if err := statusError("load", int32(status)); err != nil {
			// Callers: credentialstore.Get. Apple `security` can decrypt items
			// whose ACL/partition rejects ad-hoc Go binaries (-25293).
			// User: "load via normal companion credential-store Get without -25293."
			if errors.Is(err, ErrAuthFailed) {
				return securityCLIGet(ctx, service, account)
			}
			return nil, err
		}
		defer C.SecKeychainItemFreeContent(nil, secret)
		return C.GoBytes(secret, C.int(secretLength)), nil
	case "delete-generic-password":
		status := C.operator_keychain_delete(
			svc, C.UInt32(len(service)), acct, C.UInt32(len(account)),
		)
		if err := statusError("delete", int32(status)); err != nil {
			if errors.Is(err, ErrAuthFailed) {
				qStatus := C.operator_keychain_delete_query(svc, acct)
				return nil, statusError("delete", int32(qStatus))
			}
			return nil, err
		}
		return nil, nil
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

func statusError(operation string, status int32) error {
	if status == int32(C.errSecSuccess) {
		return nil
	}
	if status == int32(C.errSecItemNotFound) {
		return fmt.Errorf("native keychain %s: the specified item could not be found", operation)
	}
	if status == statusInteractionNotAllowed {
		return fmt.Errorf("native keychain %s: %w", operation, ErrInteractionNotAllowed)
	}
	// errSecAuthFailed (-25293): decrypt ACL rejects this binary (stale cdhash
	// or apple-tool partition). Re-Put with any-app ACL repairs it.
	if status == statusAuthFailed {
		return fmt.Errorf("native keychain %s: %w", operation, ErrAuthFailed)
	}
	return fmt.Errorf("native keychain %s failed with status %d", operation, status)
}
