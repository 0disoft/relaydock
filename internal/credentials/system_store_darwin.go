//go:build darwin

package credentials

/*
#cgo LDFLAGS: -framework CoreFoundation -framework Security

#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

static CFStringRef relaydockString(const void *bytes, size_t length) {
	return CFStringCreateWithBytes(
		kCFAllocatorDefault,
		(const UInt8 *)bytes,
		(CFIndex)length,
		kCFStringEncodingUTF8,
		false
	);
}

static CFMutableDictionaryRef relaydockCredentialQuery(CFStringRef service) {
	CFMutableDictionaryRef query = CFDictionaryCreateMutable(
		kCFAllocatorDefault,
		0,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks
	);
	if (query == NULL) {
		return NULL;
	}
	CFDictionarySetValue(query, kSecClass, kSecClassGenericPassword);
	CFDictionarySetValue(query, kSecAttrService, service);
	CFDictionarySetValue(query, kSecAttrAccount, CFSTR("RelayDock"));
	return query;
}

static OSStatus relaydockKeychainWrite(
	const void *serviceBytes,
	size_t serviceLength,
	void *secretBytes,
	size_t secretLength
) {
	CFStringRef service = relaydockString(serviceBytes, serviceLength);
	if (service == NULL) {
		return errSecParam;
	}
	CFDataRef secret = CFDataCreateWithBytesNoCopy(
		kCFAllocatorDefault,
		(const UInt8 *)secretBytes,
		(CFIndex)secretLength,
		kCFAllocatorNull
	);
	if (secret == NULL) {
		CFRelease(service);
		return errSecAllocate;
	}
	CFMutableDictionaryRef query = relaydockCredentialQuery(service);
	CFMutableDictionaryRef update = CFDictionaryCreateMutable(
		kCFAllocatorDefault,
		0,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks
	);
	if (query == NULL || update == NULL) {
		if (query != NULL) CFRelease(query);
		if (update != NULL) CFRelease(update);
		CFRelease(secret);
		CFRelease(service);
		return errSecAllocate;
	}
	CFDictionarySetValue(update, kSecValueData, secret);
	OSStatus status = SecItemUpdate(query, update);
	if (status == errSecItemNotFound) {
		CFDictionarySetValue(query, kSecValueData, secret);
		status = SecItemAdd(query, NULL);
		if (status == errSecDuplicateItem) {
			CFDictionaryRemoveValue(query, kSecValueData);
			status = SecItemUpdate(query, update);
		}
	}
	CFRelease(update);
	CFRelease(query);
	CFRelease(secret);
	CFRelease(service);
	return status;
}

static OSStatus relaydockKeychainRead(
	const void *serviceBytes,
	size_t serviceLength,
	size_t maximumLength,
	void **secretBytes,
	size_t *secretLength
) {
	*secretBytes = NULL;
	*secretLength = 0;
	CFStringRef service = relaydockString(serviceBytes, serviceLength);
	if (service == NULL) {
		return errSecParam;
	}
	CFMutableDictionaryRef query = relaydockCredentialQuery(service);
	if (query == NULL) {
		CFRelease(service);
		return errSecAllocate;
	}
	CFDictionarySetValue(query, kSecReturnData, kCFBooleanTrue);
	CFDictionarySetValue(query, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef result = NULL;
	OSStatus status = SecItemCopyMatching(query, &result);
	if (status == errSecSuccess) {
		if (result == NULL || CFGetTypeID(result) != CFDataGetTypeID()) {
			status = errSecDecode;
		} else {
			CFIndex length = CFDataGetLength((CFDataRef)result);
			if (length <= 0 || (size_t)length > maximumLength) {
				status = errSecDataTooLarge;
			} else {
				void *copy = malloc((size_t)length);
				if (copy == NULL) {
					status = errSecAllocate;
				} else {
					memcpy(copy, CFDataGetBytePtr((CFDataRef)result), (size_t)length);
					*secretBytes = copy;
					*secretLength = (size_t)length;
				}
			}
		}
	}
	if (result != NULL) CFRelease(result);
	CFRelease(query);
	CFRelease(service);
	return status;
}

static OSStatus relaydockKeychainDelete(const void *serviceBytes, size_t serviceLength) {
	CFStringRef service = relaydockString(serviceBytes, serviceLength);
	if (service == NULL) {
		return errSecParam;
	}
	CFMutableDictionaryRef query = relaydockCredentialQuery(service);
	if (query == NULL) {
		CFRelease(service);
		return errSecAllocate;
	}
	OSStatus status = SecItemDelete(query);
	CFRelease(query);
	CFRelease(service);
	return status;
}

static void relaydockFreeSecret(void *bytes, size_t length) {
	if (bytes == NULL) return;
	volatile unsigned char *cursor = (volatile unsigned char *)bytes;
	while (length > 0) {
		*cursor++ = 0;
		length--;
	}
	free(bytes);
}
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/0disoft/relaydock/internal/core"
)

type darwinCredentialBackend struct{}

func newPlatformCredentialBackend() (systemCredentialBackend, error) {
	return darwinCredentialBackend{}, nil
}

func (darwinCredentialBackend) Write(target string, value []byte) error {
	targetBytes := C.CBytes([]byte(target))
	defer C.free(targetBytes)
	secret := C.CBytes(value)
	defer C.relaydockFreeSecret(secret, C.size_t(len(value)))
	status := C.relaydockKeychainWrite(
		targetBytes, C.size_t(len(target)),
		secret, C.size_t(len(value)),
	)
	return darwinCredentialStatus("write", status)
}

func (darwinCredentialBackend) Read(target string) ([]byte, error) {
	targetBytes := C.CBytes([]byte(target))
	defer C.free(targetBytes)
	var secret unsafe.Pointer
	var length C.size_t
	status := C.relaydockKeychainRead(
		targetBytes, C.size_t(len(target)),
		C.size_t(MaximumSystemCredentialBytes), &secret, &length,
	)
	if status != C.errSecSuccess {
		return nil, darwinCredentialStatus("read", status)
	}
	defer C.relaydockFreeSecret(secret, length)
	return C.GoBytes(secret, C.int(length)), nil
}

func (darwinCredentialBackend) Delete(target string) error {
	targetBytes := C.CBytes([]byte(target))
	defer C.free(targetBytes)
	status := C.relaydockKeychainDelete(targetBytes, C.size_t(len(target)))
	return darwinCredentialStatus("delete", status)
}

func darwinCredentialStatus(operation string, status C.OSStatus) error {
	if status == C.errSecSuccess {
		return nil
	}
	if status == C.errSecItemNotFound {
		return core.ErrNotFound
	}
	return fmt.Errorf("macOS Keychain %s failed with status %d", operation, int32(status))
}
