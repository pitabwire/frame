package frame

import (
	"context"
	"os"
	"testing"
)

func TestAuthorizationControlListWrite(t *testing.T) {

	os.Setenv("KETO_AUTHORIZATION_WRITE_URL", "http://localhost:4467")

	ctx := context.Background()
	srv := NewService( "Test Srv")
	ctx = ToContext(ctx, srv)

	err := AuthorizationControlListWrite(ctx,  "read", "tested")
	if err != nil {
			t.Errorf("Authorization write was not possible see %v", err)
	}


}

func TestAuthorizationControlListHasAccess(t *testing.T) {

	os.Setenv("KETO_AUTHORIZATION_READ_URL", "http://localhost:4466")

	ctx := context.Background()
	srv := NewService( "Test Srv")
	ctx = ToContext(ctx, srv)

	err := AuthorizationControlListWrite(ctx,  "read", "reader")
	if err != nil {
		t.Errorf("Authorization write was not possible see %v", err)
	}

	err, access := AuthorizationControlListHasAccess(ctx,  "read", "reader")
	if err != nil {
		t.Errorf("Authorization check was not possible see %v", err)
	}else if !access {
		t.Errorf("Authorization check was forbidden")
	}

	err, access = AuthorizationControlListHasAccess(ctx,  "read", "read-master")
	if err != nil {
		t.Errorf("Authorization check was not possible see %v", err)
	}else if access {
		t.Errorf("Authorization check was not forbidden yet shouldn't exist")
	}

}
