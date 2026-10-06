/**
 * Tree-shaken ECharts entry point.
 *
 * Importing the full `echarts` package pulls in about a megabyte of chart
 * types this dashboard never draws. Only the detail view charts here — the
 * wall tiles use plain SVG sparklines — so the registration list is short.
 */
import * as echarts from 'echarts/core'
import { BarChart, LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  MarkLineComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([
  LineChart,
  BarChart,
  GridComponent,
  TooltipComponent,
  MarkLineComponent,
  CanvasRenderer,
])

// Type-only re-export: erased at build time, costs nothing at runtime.
export type { EChartsOption } from 'echarts'
export type ECharts = echarts.ECharts

export { echarts }
