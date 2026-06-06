import { httpClient } from '@/lib/http-client'

export type Metric = {
  value: number
  changeText: string
  changePct: number
}

export type ChartPoint = {
  name: string
  total?: number
  clicks?: number
  uniques?: number
}

export type BarItem = {
  name: string
  value: number
}

export type RecentSale = {
  name: string
  email: string
  amount: number
}

export type DashboardSnapshot = {
  overview: {
    totalRevenue: Metric
    subscriptions: Metric
    sales: Metric
    activeNow: Metric
    monthly: ChartPoint[]
    recentSales: RecentSale[]
  }
  analytics: {
    traffic: ChartPoint[]
    totalClicks: Metric
    uniqueVisitors: Metric
    bounceRate: Metric
    avgSession: Metric
    referrers: BarItem[]
    devices: BarItem[]
  }
}

export const emptyDashboardSnapshot: DashboardSnapshot = {
  overview: {
    totalRevenue: { value: 0, changeText: '暂无数据', changePct: 0 },
    subscriptions: { value: 0, changeText: '暂无数据', changePct: 0 },
    sales: { value: 0, changeText: '暂无数据', changePct: 0 },
    activeNow: { value: 0, changeText: '暂无数据', changePct: 0 },
    monthly: [
      '1月',
      '2月',
      '3月',
      '4月',
      '5月',
      '6月',
      '7月',
      '8月',
      '9月',
      '10月',
      '11月',
      '12月',
    ].map((name) => ({ name, total: 0 })),
    recentSales: [],
  },
  analytics: {
    traffic: ['周一', '周二', '周三', '周四', '周五', '周六', '周日'].map(
      (name) => ({ name, clicks: 0, uniques: 0 })
    ),
    totalClicks: { value: 0, changeText: '暂无数据', changePct: 0 },
    uniqueVisitors: { value: 0, changeText: '暂无数据', changePct: 0 },
    bounceRate: { value: 0, changeText: '暂无数据', changePct: 0 },
    avgSession: { value: 0, changeText: '暂无数据', changePct: 0 },
    referrers: [],
    devices: [],
  },
}

export async function getDashboardSnapshot(): Promise<DashboardSnapshot> {
  const response = await httpClient.get<DashboardSnapshot>('/dashboard')
  return response.data
}
