package main

import (
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/outlook"
)

func TestValidateMicrosoftAccountAcceptsMailOrPrincipalName(t *testing.T) {
	identity := outlook.AccountIdentity{Mail: "ssdear@gmail.com", UserPrincipalName: "personal_alias@outlook.com"}
	if err := validateMicrosoftAccount(identity, "ssdear@gmail.com"); err != nil {
		t.Fatalf("validate mail: %v", err)
	}
	identity = outlook.AccountIdentity{UserPrincipalName: "ssdear@gmail.com"}
	if err := validateMicrosoftAccount(identity, "ssdear@gmail.com"); err != nil {
		t.Fatalf("validate principal: %v", err)
	}
}

func TestValidateMicrosoftAccountRefusesDifferentAccount(t *testing.T) {
	identity := outlook.AccountIdentity{Mail: "raushan2@illinois.edu", UserPrincipalName: "raushan2@illinois.edu"}
	if err := validateMicrosoftAccount(identity, "ssdear@gmail.com"); err == nil {
		t.Fatal("validate different Microsoft account returned nil")
	}
}
