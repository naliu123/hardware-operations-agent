//go:build !linux

package sandbox

import "context"

func Serve(context.Context, Config) error { return ErrUnavailable }
func RunJob(string, string) error         { return ErrUnavailable }
func Probe(context.Context, Config) (Capability, error) {
	return Capability{}, ErrUnavailable
}
