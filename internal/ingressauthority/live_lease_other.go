//go:build !unix

package ingressauthority

import "context"

type liveLeaseState struct{}

func (*LiveLease) WithLockedIngress(context.Context, CommandV1, func() error) error {
	return ErrUnverified
}

func checkLiveLease(*LiveLease, context.Context, CommandV1) error { return ErrUnverified }
func (*LiveLease) Close() error                                   { return ErrUnverified }
