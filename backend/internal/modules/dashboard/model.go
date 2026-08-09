package dashboard

type Metric struct {
	Value      int64   `json:"value"`
	ChangeText string  `json:"changeText"`
	ChangePct  float64 `json:"changePct"`
}

type ChartPoint struct {
	Name    string `json:"name"`
	Total   int64  `json:"total,omitempty"`
	Clicks  int64  `json:"clicks,omitempty"`
	Uniques int64  `json:"uniques,omitempty"`
}

type BarItem struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

type RecentSale struct {
	Name   string `json:"name"`
	Email  string `json:"email"`
	Amount int64  `json:"amount"`
}

type Overview struct {
	TotalRevenue  Metric       `json:"totalRevenue"`
	Subscriptions Metric       `json:"subscriptions"`
	Sales         Metric       `json:"sales"`
	ActiveNow     Metric       `json:"activeNow"`
	Monthly       []ChartPoint `json:"monthly"`
	RecentSales   []RecentSale `json:"recentSales"`
}

type Analytics struct {
	Traffic        []ChartPoint `json:"traffic"`
	TotalClicks    Metric       `json:"totalClicks"`
	UniqueVisitors Metric       `json:"uniqueVisitors"`
	BounceRate     Metric       `json:"bounceRate"`
	AvgSession     Metric       `json:"avgSession"`
	Referrers      []BarItem    `json:"referrers"`
	Devices        []BarItem    `json:"devices"`
}

type Snapshot struct {
	Overview  Overview  `json:"overview"`
	Analytics Analytics `json:"analytics"`
}
