//go:build !unix

package ingressmanifest

import "errors"

type Store struct{}

func Open(string) (*Store, error) {
	return nil, errors.New("private ingress manifest stores require Unix ownership checks and process locks")
}
func (*Store) Close() error { return nil }
func (*Store) Prepare(Spec, *Proof) (Proof, error) {
	return Proof{}, errors.New("unsupported ingress manifest store platform")
}
func (*Store) Inspect(Proof) (Inspection, error) {
	return Inspection{}, errors.New("unsupported ingress manifest store platform")
}
