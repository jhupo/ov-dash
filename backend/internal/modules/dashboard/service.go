package dashboard

import "context"

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Snapshot(context.Context) (Snapshot, error) {
	return Snapshot{
		Overview: Overview{
			TotalRevenue:  Metric{ChangeText: "暂无数据"},
			Subscriptions: Metric{ChangeText: "暂无数据"},
			Sales:         Metric{ChangeText: "暂无数据"},
			ActiveNow:     Metric{ChangeText: "暂无数据"},
			Monthly: []ChartPoint{
				{Name: "1月"}, {Name: "2月"}, {Name: "3月"}, {Name: "4月"},
				{Name: "5月"}, {Name: "6月"}, {Name: "7月"}, {Name: "8月"},
				{Name: "9月"}, {Name: "10月"}, {Name: "11月"}, {Name: "12月"},
			},
			RecentSales: []RecentSale{},
		},
		Analytics: Analytics{
			Traffic: []ChartPoint{
				{Name: "周一"}, {Name: "周二"}, {Name: "周三"}, {Name: "周四"},
				{Name: "周五"}, {Name: "周六"}, {Name: "周日"},
			},
			TotalClicks:    Metric{ChangeText: "暂无数据"},
			UniqueVisitors: Metric{ChangeText: "暂无数据"},
			BounceRate:     Metric{ChangeText: "暂无数据"},
			AvgSession:     Metric{ChangeText: "暂无数据"},
			Referrers:      []BarItem{},
			Devices:        []BarItem{},
		},
	}, nil
}
