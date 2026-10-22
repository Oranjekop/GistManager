package security

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestWindowsHelloRejectsClosedOwnerAndCancelledContext(t *testing.T) {
	if err := (SystemBiometric{}).VerifyForWindow(context.Background(), ^uintptr(0)); err == nil {
		t.Fatal("invalid parent window accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runHello(ctx, true, 0); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled verification attempted to display a prompt", err)
	}
}

// Opt-in native API check; never displays a credential prompt.
func TestWindowsHelloNativeAvailability(t *testing.T) {
	if os.Getenv("GIST_HELLO_CHECK") != "1" {
		t.Skip("set GIST_HELLO_CHECK=1 to inspect the local WinRT interfaces")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := runHello(ctx, false, 0); err != nil {
		t.Fatal("native availability operation failed", err)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	result, _, _ := roInitialize.Call(1)
	if err := helloHRESULT(result); err != nil {
		t.Fatal(err)
	}
	defer roUninitialize.Call()
	factory, err := consentFactory(&iidConsentInterop)
	if err != nil {
		t.Fatal("desktop window interop unavailable", err)
	}
	factory.release()
}
