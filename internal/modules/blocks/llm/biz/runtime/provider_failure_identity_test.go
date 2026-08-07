package runtime

import "testing"

func TestProviderFailureIdentityClassifiesIdleAndTransportTimeout(t *testing.T) {
	code, signature := providerFailureIdentity(assertError("upstream response body idle timeout after 1m0s"))
	if code != "provider_body_idle_timeout" || signature != "provider-body-idle-timeout" {
		t.Fatalf("idle timeout identity = %q/%q", code, signature)
	}
	code, signature = providerFailureIdentity(assertError("llm provider request timeout: context deadline exceeded"))
	if code != "provider_transport_timeout" || signature != "provider-transport-timeout" {
		t.Fatalf("transport timeout identity = %q/%q", code, signature)
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }
