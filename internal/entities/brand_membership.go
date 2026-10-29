package entities

import "time"

type BrandRole string

const (
	BrandOwner  BrandRole = "owner"
	BrandEditor BrandRole = "editor"
)

type MembershipStatus string

const (
	MembershipInvited  MembershipStatus = "invited"
	MembershipActive   MembershipStatus = "active"
	MembershipRevoked  MembershipStatus = "revoked"
	MembershipDeclined MembershipStatus = "declined"
)

type BrandMembershipState struct {
	UserID, BrandID      int64
	Role                 BrandRole
	Status               MembershipStatus
	InvitedByUserID      *int64
	InvitedAt, JoinedAt  *time.Time
	CreatedAt, UpdatedAt time.Time
}

type BrandMembership struct{ state BrandMembershipState }

func NewOwnerMembership(userID, brandID int64, now time.Time) (*BrandMembership, error) {
	s := BrandMembershipState{UserID: userID, BrandID: brandID, Role: BrandOwner, Status: MembershipActive, JoinedAt: copyTime(&now), CreatedAt: now, UpdatedAt: now}
	if err := validateMembership(s); err != nil {
		return nil, err
	}
	return &BrandMembership{state: s}, nil
}

func NewEditorInvitation(userID, brandID, invitedByUserID int64, now time.Time) (*BrandMembership, error) {
	s := BrandMembershipState{UserID: userID, BrandID: brandID, Role: BrandEditor, Status: MembershipInvited, InvitedByUserID: copyID(&invitedByUserID), InvitedAt: copyTime(&now), CreatedAt: now, UpdatedAt: now}
	if err := validateMembership(s); err != nil {
		return nil, err
	}
	return &BrandMembership{state: s}, nil
}

func cloneMembership(s BrandMembershipState) BrandMembershipState {
	s.InvitedByUserID = copyID(s.InvitedByUserID)
	s.InvitedAt = copyTime(s.InvitedAt)
	s.JoinedAt = copyTime(s.JoinedAt)
	return s
}

func RestoreBrandMembership(s BrandMembershipState) (*BrandMembership, error) {
	if err := validateMembership(s); err != nil {
		return nil, err
	}
	return &BrandMembership{state: cloneMembership(s)}, nil
}

func validateMembership(s BrandMembershipState) error {
	if s.UserID <= 0 || s.BrandID <= 0 {
		return invalid("membership.ids")
	}
	if !validTimes(s.CreatedAt, s.UpdatedAt) {
		return invalid("membership.timestamps")
	}
	if s.JoinedAt != nil && (s.JoinedAt.Before(s.CreatedAt) || s.JoinedAt.After(s.UpdatedAt)) {
		return invalid("membership.joined_at")
	}
	// Invitation metadata is optional history, independent of the current role.
	// A former owner may be an active editor without ever being invited.
	hasInvitation := s.InvitedAt != nil
	if hasInvitation != (s.InvitedByUserID != nil) {
		return invalid("membership.invitation")
	}
	if hasInvitation {
		if *s.InvitedByUserID <= 0 || s.InvitedAt.Before(s.CreatedAt) || s.InvitedAt.After(s.UpdatedAt) {
			return invalid("membership.invitation")
		}
		if s.JoinedAt != nil && s.JoinedAt.Before(*s.InvitedAt) {
			return invalid("membership.joined_at")
		}
	}
	if s.Role == BrandOwner {
		if s.Status != MembershipActive || s.JoinedAt == nil {
			return invalid("membership.owner")
		}
		return nil
	}
	if s.Role != BrandEditor {
		return invalid("membership.role")
	}
	switch s.Status {
	case MembershipInvited, MembershipDeclined:
		if !hasInvitation {
			return invalid("membership.invitation")
		}
		if s.JoinedAt != nil {
			return invalid("membership.joined_at")
		}
	case MembershipActive:
		if s.JoinedAt == nil {
			return invalid("membership.joined_at")
		}
	case MembershipRevoked:
		if !hasInvitation && s.JoinedAt == nil {
			return invalid("membership.joined_at")
		}
	default:
		return invalid("membership.status")
	}
	return nil
}

func (m *BrandMembership) change(s BrandMembershipState, now time.Time) error {
	if err := checkTime(now, m.state.UpdatedAt); err != nil {
		return err
	}
	s.UpdatedAt = now
	if err := validateMembership(s); err != nil {
		return err
	}
	m.state = cloneMembership(s)
	return nil
}

func (m *BrandMembership) Accept(now time.Time) error {
	if m.state.Role != BrandEditor || m.state.Status != MembershipInvited {
		return ErrInvalidTransition
	}
	s := m.state
	s.Status = MembershipActive
	s.JoinedAt = &now
	return m.change(s, now)
}

func (m *BrandMembership) Decline(now time.Time) error {
	if m.state.Role != BrandEditor || m.state.Status != MembershipInvited {
		return ErrInvalidTransition
	}
	s := m.state
	s.Status = MembershipDeclined
	return m.change(s, now)
}

// Revoke cannot remove an owner. Ownership transfer needs a multi-record usecase.
func (m *BrandMembership) Revoke(now time.Time) error {
	if m.state.Role != BrandEditor || (m.state.Status != MembershipInvited && m.state.Status != MembershipActive) {
		return ErrInvalidTransition
	}
	s := m.state
	s.Status = MembershipRevoked
	return m.change(s, now)
}

func (m *BrandMembership) Reinvite(invitedByUserID int64, now time.Time) error {
	if m.state.Role != BrandEditor || (m.state.Status != MembershipRevoked && m.state.Status != MembershipDeclined) {
		return ErrInvalidTransition
	}
	s := m.state
	s.Status = MembershipInvited
	s.InvitedByUserID = &invitedByUserID
	s.InvitedAt = &now
	s.JoinedAt = nil
	return m.change(s, now)
}

func (m BrandMembership) State() BrandMembershipState { return cloneMembership(m.state) }
func (m BrandMembership) UserID() int64               { return m.state.UserID }
func (m BrandMembership) BrandID() int64              { return m.state.BrandID }
func (m BrandMembership) Role() BrandRole             { return m.state.Role }
func (m BrandMembership) Status() MembershipStatus    { return m.state.Status }
func (m BrandMembership) InvitedByUserID() *int64     { return copyID(m.state.InvitedByUserID) }
func (m BrandMembership) InvitedAt() *time.Time       { return copyTime(m.state.InvitedAt) }
func (m BrandMembership) JoinedAt() *time.Time        { return copyTime(m.state.JoinedAt) }
func (m BrandMembership) CreatedAt() time.Time        { return m.state.CreatedAt }
func (m BrandMembership) UpdatedAt() time.Time        { return m.state.UpdatedAt }
