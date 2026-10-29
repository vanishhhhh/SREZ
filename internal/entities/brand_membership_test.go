package entities_test

import (
	"srez/internal/entities"
	"testing"
	"time"
)

func editor(t *testing.T) *entities.BrandMembership {
	t.Helper()
	m, err := entities.NewEditorInvitation(1, 2, 3, created)
	noError(t, err)
	return m
}

func TestNewMembership(t *testing.T) {
	for _, tt := range []struct {
		name                 string
		user, brand, inviter int64
		owner, valid         bool
		now                  time.Time
	}{
		{"owner", 1, 2, 0, true, true, created}, {"editor", 1, 2, 3, false, true, created},
		{"owner user", 0, 2, 0, true, false, created}, {"owner brand", 1, -1, 0, true, false, created},
		{"editor user", -1, 2, 3, false, false, created}, {"editor brand", 1, 0, 3, false, false, created},
		{"inviter", 1, 2, 0, false, false, created}, {"owner time", 1, 2, 0, true, false, time.Time{}}, {"editor time", 1, 2, 3, false, false, time.Time{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var m *entities.BrandMembership
			var err error
			if tt.owner {
				m, err = entities.NewOwnerMembership(tt.user, tt.brand, tt.now)
			} else {
				m, err = entities.NewEditorInvitation(tt.user, tt.brand, tt.inviter, tt.now)
			}
			if !tt.valid {
				if err == nil || m != nil {
					t.Fatal("invalid membership accepted")
				}
				return
			}
			noError(t, err)
			same(t, m.UserID(), tt.user)
			same(t, m.BrandID(), tt.brand)
			same(t, m.CreatedAt(), created)
			same(t, m.UpdatedAt(), created)
			if tt.owner {
				same(t, m.Role(), entities.BrandOwner)
				same(t, m.Status(), entities.MembershipActive)
				same(t, *m.JoinedAt(), created)
				same(t, m.InvitedAt(), (*time.Time)(nil))
				same(t, m.InvitedByUserID(), (*int64)(nil))
			} else {
				same(t, m.Role(), entities.BrandEditor)
				same(t, m.Status(), entities.MembershipInvited)
				same(t, *m.InvitedAt(), created)
				same(t, *m.InvitedByUserID(), int64(3))
				same(t, m.JoinedAt(), (*time.Time)(nil))
			}
		})
	}
}

func TestMembershipTransitions(t *testing.T) {
	for _, role := range []entities.BrandRole{entities.BrandOwner, entities.BrandEditor} {
		for _, status := range []entities.MembershipStatus{entities.MembershipInvited, entities.MembershipActive, entities.MembershipRevoked, entities.MembershipDeclined} {
			if role == entities.BrandOwner && status != entities.MembershipActive {
				continue
			}
			for _, action := range []string{"accept", "decline", "revoke", "reinvite"} {
				t.Run(string(role)+"/"+string(status)+"/"+action, func(t *testing.T) {
					m := editor(t)
					if role == entities.BrandOwner {
						var err error
						m, err = entities.NewOwnerMembership(1, 2, created)
						noError(t, err)
					} else {
						s := m.State()
						s.Status = status
						if status == entities.MembershipActive {
							s.JoinedAt = &created
						}
						var err error
						m, err = entities.RestoreBrandMembership(s)
						noError(t, err)
					}
					before := m.State()
					want := status
					valid := false
					var err error
					switch action {
					case "accept":
						valid = status == entities.MembershipInvited
						want = entities.MembershipActive
						err = m.Accept(changed)
					case "decline":
						valid = status == entities.MembershipInvited
						want = entities.MembershipDeclined
						err = m.Decline(changed)
					case "revoke":
						valid = status == entities.MembershipInvited || status == entities.MembershipActive
						want = entities.MembershipRevoked
						err = m.Revoke(changed)
					case "reinvite":
						valid = status == entities.MembershipRevoked || status == entities.MembershipDeclined
						want = entities.MembershipInvited
						err = m.Reinvite(9, changed)
					}
					valid = valid && role == entities.BrandEditor
					if !valid {
						unchanged(t, before, m.State(), err, entities.ErrInvalidTransition)
						return
					}
					noError(t, err)
					same(t, m.Status(), want)
					same(t, m.UpdatedAt(), changed)
					same(t, m.CreatedAt(), created)
					if action == "accept" {
						same(t, *m.JoinedAt(), changed)
					}
					if action == "reinvite" {
						same(t, *m.InvitedByUserID(), int64(9))
						same(t, *m.InvitedAt(), changed)
						same(t, m.JoinedAt(), (*time.Time)(nil))
					}
				})
			}
		}
	}
}

func TestMembershipReinviteAndAtomicity(t *testing.T) {
	m := editor(t)
	noError(t, m.Accept(created))
	noError(t, m.Revoke(created))
	noError(t, m.Reinvite(8, changed))
	same(t, m.Status(), entities.MembershipInvited)
	same(t, m.JoinedAt(), (*time.Time)(nil))
	before := m.State()
	err := m.Accept(created)
	unchanged(t, before, m.State(), err, entities.ErrInvalidValue)
	for _, action := range []string{"accept", "decline", "revoke", "reinvite"} {
		t.Run(action, func(t *testing.T) {
			m := editor(t)
			if action == "reinvite" {
				noError(t, m.Decline(created))
			}
			before := m.State()
			var err error
			switch action {
			case "accept":
				err = m.Accept(time.Time{})
			case "decline":
				err = m.Decline(created.Add(-time.Second))
			case "revoke":
				err = m.Revoke(time.Time{})
			case "reinvite":
				err = m.Reinvite(0, changed)
			}
			unchanged(t, before, m.State(), err, entities.ErrInvalidValue)
		})
	}
}

func TestMembershipPointerIsolation(t *testing.T) {
	s := editor(t).State()
	m, err := entities.RestoreBrandMembership(s)
	noError(t, err)
	before := m.State()
	*s.InvitedByUserID = 88
	*s.InvitedAt = changed
	got := m.State()
	*got.InvitedByUserID = 99
	*got.InvitedAt = changed
	*m.InvitedByUserID() = 99
	*m.InvitedAt() = changed
	same(t, m.State(), before)
	noError(t, m.Accept(changed))
	before = m.State()
	*m.JoinedAt() = created
	got = m.State()
	*got.JoinedAt = created
	same(t, m.State(), before)
}

func TestRestoreMembershipRejectsInconsistentState(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*entities.BrandMembershipState)
	}{
		{"role", func(s *entities.BrandMembershipState) { s.Role = "bad" }},
		{"status", func(s *entities.BrandMembershipState) { s.Status = "bad" }},
		{"inviter", func(s *entities.BrandMembershipState) { s.InvitedByUserID = nil }},
		{"invited time", func(s *entities.BrandMembershipState) { s.InvitedAt = nil }},
		{"active without joined", func(s *entities.BrandMembershipState) { s.Status = entities.MembershipActive }},
		{"joined while invited", func(s *entities.BrandMembershipState) { s.JoinedAt = &created }},
		{"future invitation", func(s *entities.BrandMembershipState) { s.InvitedAt = &changed }},
		{"old updated", func(s *entities.BrandMembershipState) { s.UpdatedAt = created.Add(-time.Second) }},
		{"invalid owner", func(s *entities.BrandMembershipState) { s.Role = entities.BrandOwner }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := editor(t).State()
			tt.mutate(&s)
			if _, err := entities.RestoreBrandMembership(s); err == nil {
				t.Fatal("invalid membership accepted")
			}
		})
	}
}

func TestFormerOwnerAsActiveEditor(t *testing.T) {
	owner, err := entities.NewOwnerMembership(1, 2, created)
	noError(t, err)
	ownerBefore := owner.State()
	// A future transactional usecase persists this role change together with
	// the new owner. Here we test representation and local invariants only.
	s := owner.State()
	s.Role = entities.BrandEditor
	s.UpdatedAt = changed
	m, err := entities.RestoreBrandMembership(s)
	noError(t, err)
	same(t, owner.State(), ownerBefore)
	same(t, m.Role(), entities.BrandEditor)
	same(t, m.Status(), entities.MembershipActive)
	same(t, *m.JoinedAt(), created)
	same(t, m.InvitedAt(), (*time.Time)(nil))
	same(t, m.InvitedByUserID(), (*int64)(nil))
	same(t, m.CreatedAt(), created)
	same(t, m.UpdatedAt(), changed)
	before := m.State()
	err = m.Accept(changed)
	unchanged(t, before, m.State(), err, entities.ErrInvalidTransition)
	err = m.Reinvite(3, changed)
	unchanged(t, before, m.State(), err, entities.ErrInvalidTransition)
	noError(t, m.Revoke(changed))
	same(t, m.Status(), entities.MembershipRevoked)
	same(t, m.InvitedAt(), (*time.Time)(nil))
	same(t, *m.JoinedAt(), created)
	restored, err := entities.RestoreBrandMembership(m.State())
	noError(t, err)
	same(t, restored.State(), m.State())
	before = m.State()
	err = m.Reinvite(0, changed.Add(time.Hour))
	unchanged(t, before, m.State(), err, entities.ErrInvalidValue)
	noError(t, m.Reinvite(3, changed.Add(time.Hour)))
	same(t, m.Status(), entities.MembershipInvited)
	same(t, m.JoinedAt(), (*time.Time)(nil))
	same(t, *m.InvitedByUserID(), int64(3))
	same(t, *m.InvitedAt(), changed.Add(time.Hour))
	noError(t, m.Accept(changed.Add(2*time.Hour)))
	same(t, m.Status(), entities.MembershipActive)
}

func TestMembershipOptionalInvitationHistory(t *testing.T) {
	for _, tt := range []struct {
		name   string
		role   entities.BrandRole
		status entities.MembershipStatus
		joined bool
		valid  bool
	}{
		{"owner", entities.BrandOwner, entities.MembershipActive, true, true},
		{"former owner", entities.BrandEditor, entities.MembershipActive, true, true},
		{"revoked former owner", entities.BrandEditor, entities.MembershipRevoked, true, true},
		{"active needs joined", entities.BrandEditor, entities.MembershipActive, false, false},
		{"revoked needs history", entities.BrandEditor, entities.MembershipRevoked, false, false},
		{"invited needs invitation", entities.BrandEditor, entities.MembershipInvited, false, false},
		{"declined needs invitation", entities.BrandEditor, entities.MembershipDeclined, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := entities.BrandMembershipState{UserID: 1, BrandID: 2, Role: tt.role, Status: tt.status, CreatedAt: created, UpdatedAt: changed}
			if tt.joined {
				joined := created
				s.JoinedAt = &joined
			}
			m, err := entities.RestoreBrandMembership(s)
			if !tt.valid {
				if err == nil || m != nil {
					t.Fatal("inconsistent membership accepted")
				}
				return
			}
			noError(t, err)
			same(t, m.State(), s)
		})
	}
	for _, missing := range []string{"inviter", "invited_at"} {
		t.Run("partial history/"+missing, func(t *testing.T) {
			m := editor(t)
			noError(t, m.Accept(changed))
			s := m.State()
			if missing == "inviter" {
				s.InvitedByUserID = nil
			} else {
				s.InvitedAt = nil
			}
			if _, err := entities.RestoreBrandMembership(s); err == nil {
				t.Fatal("partial invitation history accepted")
			}
		})
	}
}

func TestOwnerCanRetainRealInvitationHistory(t *testing.T) {
	m := editor(t)
	noError(t, m.Accept(changed))
	s := m.State()
	s.Role = entities.BrandOwner
	owner, err := entities.RestoreBrandMembership(s)
	noError(t, err)
	same(t, owner.State(), s)
	before := owner.State()
	err = owner.Revoke(changed)
	unchanged(t, before, owner.State(), err, entities.ErrInvalidTransition)
}
