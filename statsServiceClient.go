package v2ray_ssrpanel_plugin

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"strings"
	statsservice "v2ray.com/core/app/stats/command"
)

type StatsServiceClient struct {
	statsservice.StatsServiceClient
}

func NewStatsServiceClient(client *grpc.ClientConn) *StatsServiceClient {
	return &StatsServiceClient{
		StatsServiceClient: statsservice.NewStatsServiceClient(client),
	}
}

func (s *StatsServiceClient) getUserUplink(email string) (uint64, error) {
	return s.getUserTraffic(fmt.Sprintf("user>>>%s>>>traffic>>>uplink", email), true)
}

func (s *StatsServiceClient) getUserDownlink(email string) (uint64, error) {
	return s.getUserTraffic(fmt.Sprintf("user>>>%s>>>traffic>>>downlink", email), true)
}

func (s *StatsServiceClient) getUserTraffic(name string, reset bool) (uint64, error) {
	req := &statsservice.GetStatsRequest{
		Name:   name,
		Reset_: reset,
	}

	res, err := s.GetStats(context.Background(), req)
	if err != nil {
		if status, ok := status.FromError(err); ok && strings.HasSuffix(status.Message(), fmt.Sprintf("%s not found.", name)) {
			return 0, nil
		}

		return 0, err
	}

	return uint64(res.Stat.Value), nil
}

func (s *StatsServiceClient) getUserIPStats(email string, reset bool) (string, error) {
	name := fmt.Sprintf("user>>>%s>>>ip", email)
	req := &statsservice.GetStatsRequest{
		Name:   name,
		Reset_: reset,
	}

	res, err := s.StatsServiceClient.GetUserIPStats(context.Background(), req)
	if err != nil {
		if status, ok := status.FromError(err); ok && strings.HasSuffix(status.Message(), fmt.Sprintf("%s not found.", name)) {
			return "", nil
		}

		return "", err
	}

	return res.Value, nil
}
