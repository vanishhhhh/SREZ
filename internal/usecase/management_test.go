package usecase

import (
	"context"
	"errors"
	"srez/internal/entities"
	"testing"
	"time"
)

type permissionTx struct {
	ManagementTx
	member *entities.BrandMembership
}

func (t permissionTx) Membership(context.Context, int64) (*entities.BrandMembership, error) {
	return t.member, nil
}
func TestBrandRolePolicy(t *testing.T) {
	now := time.Now().UTC()
	owner, e := entities.NewOwnerMembership(1, 1, now)
	if e != nil {
		t.Fatal(e)
	}
	invited, e := entities.NewEditorInvitation(2, 1, 1, now)
	if e != nil {
		t.Fatal(e)
	}
	active := *invited
	if e = active.Accept(now); e != nil {
		t.Fatal(e)
	}
	revoked := active
	if e = revoked.Revoke(now); e != nil {
		t.Fatal(e)
	}
	declined := *invited
	if e = declined.Decline(now); e != nil {
		t.Fatal(e)
	}
	for _, tt := range []struct {
		name      string
		member    *entities.BrandMembership
		ownerOnly bool
		want      error
	}{{"owner", owner, true, nil}, {"editor", &active, false, nil}, {"editor settings", &active, true, ErrForbidden}, {"guest", nil, false, ErrResourceHidden}, {"invited", invited, false, ErrResourceHidden}, {"revoked", &revoked, false, ErrResourceHidden}, {"declined", &declined, false, ErrResourceHidden}} {
		t.Run(tt.name, func(t *testing.T) {
			e := authorize(context.Background(), permissionTx{member: tt.member}, 1, tt.ownerOnly)
			if !errors.Is(e, tt.want) {
				t.Fatal(e)
			}
		})
	}
}
