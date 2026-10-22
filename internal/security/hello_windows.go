package security

import (
	"context"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

var (
	combase             = syscall.NewLazyDLL("combase.dll")
	roInitialize        = combase.NewProc("RoInitialize")
	roUninitialize      = combase.NewProc("RoUninitialize")
	roGetFactory        = combase.NewProc("RoGetActivationFactory")
	windowsCreateString = combase.NewProc("WindowsCreateString")
	windowsDeleteString = combase.NewProc("WindowsDeleteString")
	isWindow            = syscall.NewLazyDLL("user32.dll").NewProc("IsWindow")

	iidConsentStatics = syscall.GUID{Data1: 0xaf4f3f91, Data2: 0x564c, Data3: 0x4ddc, Data4: [8]byte{0xb8, 0xb5, 0x97, 0x34, 0x47, 0x62, 0x7c, 0x65}}
	iidConsentInterop = syscall.GUID{Data1: 0x39e050c3, Data2: 0x4e74, Data3: 0x441a, Data4: [8]byte{0x8d, 0xc0, 0xb8, 0x11, 0x04, 0xdf, 0x94, 0x9c}}
	iidConsentResult  = syscall.GUID{Data1: 0xfd596ffd, Data2: 0x2318, Data3: 0x558f, Data4: [8]byte{0x9d, 0xbe, 0xd2, 0x1d, 0xf4, 0x37, 0x64, 0xa5}}
	iidAsyncInfo      = syscall.GUID{Data1: 0x00000036, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
)

type winRTObject struct {
	vtable *[11]uintptr
}

func helloHRESULT(result uintptr) error {
	if int32(result) < 0 {
		return fmt.Errorf("Windows Hello 接口错误 (0x%08X)", uint32(result))
	}
	return nil
}

func (o *winRTObject) release() {
	syscall.SyscallN(o.vtable[2], uintptr(unsafe.Pointer(o)))
}

func helloString(value string) (uintptr, error) {
	units, err := syscall.UTF16FromString(value)
	if err != nil {
		return 0, err
	}
	var handle uintptr
	result, _, _ := windowsCreateString.Call(uintptr(unsafe.Pointer(&units[0])), uintptr(len(units)-1), uintptr(unsafe.Pointer(&handle)))
	return handle, helloHRESULT(result)
}

func consentFactory(iid *syscall.GUID) (*winRTObject, error) {
	class, err := helloString("Windows.Security.Credentials.UI.UserConsentVerifier")
	if err != nil {
		return nil, err
	}
	defer windowsDeleteString.Call(class)
	var factory *winRTObject
	result, _, _ := roGetFactory.Call(class, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&factory)))
	return factory, helloHRESULT(result)
}

func runHello(ctx context.Context, verify bool, owner uintptr) (int32, error) {
	ctx, cancel := context.WithTimeout(ctx, 65*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if verify && owner != 0 {
		valid, _, _ := isWindow.Call(owner)
		if valid == 0 {
			return 0, fmt.Errorf("Windows Hello 的所属窗口已关闭")
		}
	}
	// WinRT initialization and every COM call stay in the same MTA apartment.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	result, _, _ := roInitialize.Call(1)
	if err := helloHRESULT(result); err != nil {
		return 0, err
	}
	defer roUninitialize.Call()

	iid := &iidConsentStatics
	if verify && owner != 0 {
		iid = &iidConsentInterop
	}
	factory, err := consentFactory(iid)
	if err != nil {
		return 0, err
	}
	defer factory.release()
	var operation *winRTObject
	if !verify {
		// IUserConsentVerifierStatics inherits IInspectable (six base slots).
		result, _, _ = syscall.SyscallN(factory.vtable[6], uintptr(unsafe.Pointer(factory)), uintptr(unsafe.Pointer(&operation)))
	} else {
		message, stringErr := helloString("验证身份以解锁或管理 Gist管理器密码")
		if stringErr != nil {
			return 0, stringErr
		}
		defer windowsDeleteString.Call(message)
		if owner != 0 {
			// IUserConsentVerifierInterop also inherits IInspectable. The HWND
			// lets Windows position and associate its secure prompt with our app.
			result, _, _ = syscall.SyscallN(factory.vtable[6], uintptr(unsafe.Pointer(factory)), owner, message,
				uintptr(unsafe.Pointer(&iidConsentResult)), uintptr(unsafe.Pointer(&operation)))
		} else {
			result, _, _ = syscall.SyscallN(factory.vtable[7], uintptr(unsafe.Pointer(factory)), message, uintptr(unsafe.Pointer(&operation)))
		}
	}
	if err = helloHRESULT(result); err != nil {
		return 0, err
	}
	defer operation.release()
	return awaitHello(ctx, operation)
}

func awaitHello(ctx context.Context, operation *winRTObject) (int32, error) {
	var info *winRTObject
	result, _, _ := syscall.SyscallN(operation.vtable[0], uintptr(unsafe.Pointer(operation)),
		uintptr(unsafe.Pointer(&iidAsyncInfo)), uintptr(unsafe.Pointer(&info)))
	if err := helloHRESULT(result); err != nil {
		return 0, err
	}
	defer info.release()
	defer syscall.SyscallN(info.vtable[10], uintptr(unsafe.Pointer(info))) // IAsyncInfo.Close
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	for {
		var status int32
		result, _, _ = syscall.SyscallN(info.vtable[7], uintptr(unsafe.Pointer(info)), uintptr(unsafe.Pointer(&status)))
		if err := helloHRESULT(result); err != nil {
			return 0, err
		}
		if status != 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			var value int32
			result, _, _ = syscall.SyscallN(operation.vtable[8], uintptr(unsafe.Pointer(operation)), uintptr(unsafe.Pointer(&value)))
			return value, helloHRESULT(result)
		}
		select {
		case <-ctx.Done():
			_, _, _ = syscall.SyscallN(info.vtable[9], uintptr(unsafe.Pointer(info))) // IAsyncInfo.Cancel
			return 0, ctx.Err()
		case <-ticker.C:
		}
	}
}
