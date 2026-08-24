package runtime

import (
	"fmt"
	"testing"
)

func TestProviderFailureIdentityDoesNotClassifyErrorText(t *testing.T) {
	for _, message := range []string{
		"upstream response body idle timeout after 1m0s",
		"llm provider request timeout: context deadline exceeded",
		"empty response",
	} {
		code, signature := providerFailureIdentity(assertError(message))
		if code != "provider_request_failed" || signature != "provider-request-failed" {
			t.Fatalf("generic error %q identity = %q/%q", message, code, signature)
		}
	}
}

func TestProviderFailureIdentityPrefersWrappedTypedOwnerIdentity(t *testing.T) {
	typed := typedProviderIdentityError{code: " response_byte_limit_exceeded "}
	code, signature := providerFailureIdentity(fmt.Errorf("generic outer error: %w", typed))
	if code != "response_byte_limit_exceeded" || signature != "response-byte-limit-exceeded" {
		t.Fatalf("typed provider identity = %q/%q", code, signature)
	}

	typed.signature = " exact-provider-signature "
	code, signature = providerFailureIdentity(typed)
	if code != "response_byte_limit_exceeded" || signature != "exact-provider-signature" {
		t.Fatalf("explicit typed provider identity = %q/%q", code, signature)
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }

type typedProviderIdentityError struct {
	code      string
	signature string
}

func (e typedProviderIdentityError) Error() string { return "typed provider failure" }

func (e typedProviderIdentityError) RuntimeProviderFailureIdentity() (string, string) {
	return e.code, e.signature
}
