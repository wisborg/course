package main

import (
	"os"
	"testing"

	"github.com/wisborg/secrets"
)

// The tests keep the Strava login in files, as on a system with no
// keychain, and the keychain they would reach is one in memory, never the
// user's own.
func TestMain(m *testing.M) {
	secrets.MockKeychain(nil)
	os.Setenv(secrets.EnvStore, "file")
	os.Exit(m.Run())
}
