package controller_test

import (
	"context"

	monitoringv1alpha1 "github.com/siutsin/heartbeats/api/v1alpha1"
)

// fakeHealthChecker is a hand-written fake for controller.HealthChecker.
// The interface has one method and two call sites, so a generated mock
// is not worth its tool.
type fakeHealthChecker struct {
	check func(ctx context.Context, endpoint string, ranges []monitoringv1alpha1.StatusCodeRange, secret monitoringv1alpha1.EndpointsSecret) (bool, int, bool, error)
}

// CheckEndpointHealth implements controller.HealthChecker.
func (f *fakeHealthChecker) CheckEndpointHealth(ctx context.Context, endpoint string, ranges []monitoringv1alpha1.StatusCodeRange, secret monitoringv1alpha1.EndpointsSecret) (bool, int, bool, error) {
	return f.check(ctx, endpoint, ranges, secret)
}
