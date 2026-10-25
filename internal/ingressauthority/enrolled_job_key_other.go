//go:build unix && !linux

package ingressauthority

import "context"

type enrolledJobKeyIdentity struct{ Public [32]byte }

func loadEnrolledJobKey(context.Context) (enrolledJobKeyIdentity, error) {
	return enrolledJobKeyIdentity{}, ErrUnverified
}

func checkInstalledAuthorityStore(context.Context, *ReservationStore) error { return ErrUnverified }
func OpenInstalledManagedReservationStore(context.Context, string) (*ReservationStore, error) {
	return nil, ErrUnverified
}
