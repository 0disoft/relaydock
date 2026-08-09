package ssrf

import (
	"context"
	"net"
	"testing"
)

type fixedResolver struct {
	addresses []net.IPAddr
}

func (r fixedResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return r.addresses, nil
}

func TestResolveAndValidateReturnsValidatedDialAddresses(t *testing.T) {
	validator := Validator{Resolver: fixedResolver{addresses: []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}}}
	addresses, err := validator.ResolveAndValidate(context.Background(), "issuer.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(addresses) != 1 || addresses[0].String() != "203.0.113.10" {
		t.Fatalf("unexpected validated addresses: %v", addresses)
	}
}

func TestResolveAndValidateRejectsMixedPublicAndPrivateResults(t *testing.T) {
	validator := Validator{Resolver: fixedResolver{addresses: []net.IPAddr{
		{IP: net.ParseIP("203.0.113.10")}, {IP: net.ParseIP("127.0.0.1")},
	}}}
	if _, err := validator.ResolveAndValidate(context.Background(), "rebinding.example.com"); err == nil {
		t.Fatal("accepted DNS result containing a private address")
	}
}

func TestResolveAndValidateAllowsPrivateIPOnlyWhenExplicit(t *testing.T) {
	validator := Validator{AllowPrivate: true}
	addresses, err := validator.ResolveAndValidate(context.Background(), "127.0.0.1")
	if err != nil || len(addresses) != 1 {
		t.Fatalf("private development issuer rejected: %v, %v", addresses, err)
	}
	if _, err := validator.ResolveAndValidate(context.Background(), "localhost"); err == nil {
		t.Fatal("localhost alias must remain forbidden")
	}
	if _, err := validator.ResolveAndValidate(context.Background(), "169.254.169.254"); err == nil {
		t.Fatal("cloud metadata address must remain forbidden")
	}
}
