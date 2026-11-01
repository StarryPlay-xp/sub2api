<template>
  <svg
    class="block w-full overflow-visible"
    :style="{ height: `${height}px` }"
    :viewBox="`0 0 ${WIDTH} ${height}`"
    preserveAspectRatio="none"
    role="img"
    :aria-label="label"
  >
    <line
      :x1="0"
      :x2="WIDTH"
      :y1="height - 0.5"
      :y2="height - 0.5"
      class="stroke-gray-200 dark:stroke-dark-700"
      stroke-width="1"
      vector-effect="non-scaling-stroke"
    />
    <g v-for="line in lines" :key="line.key">
      <path v-if="line.area" :d="line.area" :fill="line.color" fill-opacity="0.1" />
      <path
        v-if="line.path"
        :d="line.path"
        fill="none"
        :stroke="line.color"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        vector-effect="non-scaling-stroke"
      />
    </g>
  </svg>
</template>

<script setup lang="ts">
import { computed } from 'vue'

// 轻量趋势线：只表达形状，具体数值由旁边的文字给出。null 表示无记录，断开线段而不是画成 0。
interface SparkSeries {
  key: string
  color: string
  values: (number | null)[]
  area?: boolean
}

const props = withDefaults(
  defineProps<{ series: SparkSeries[]; height?: number; label: string; max?: number | null }>(),
  { height: 48, max: null }
)

const WIDTH = 300
const PAD = 3

const scaleMax = computed(() => {
  const values = props.series.flatMap((item) => item.values.filter((value): value is number => value !== null))
  const dataMax = values.length ? Math.max(...values) : 0
  return Math.max(dataMax, props.max ?? 0, 1e-9)
})

const lines = computed(() =>
  props.series.map((item) => {
    const count = item.values.length
    const x = (index: number) => (count <= 1 ? WIDTH : (index / (count - 1)) * WIDTH)
    const y = (value: number) => props.height - PAD - (value / scaleMax.value) * (props.height - PAD * 2)
    let path = ''
    let area = ''
    let segment: [number, number][] = []
    const flush = () => {
      if (!segment.length) return
      path += segment.map(([px, py], index) => `${index ? 'L' : 'M'}${px.toFixed(2)},${py.toFixed(2)}`).join('')
      if (item.area && segment.length > 1) {
        const first = segment[0]
        const last = segment[segment.length - 1]
        area +=
          `M${first[0].toFixed(2)},${props.height}` +
          segment.map(([px, py]) => `L${px.toFixed(2)},${py.toFixed(2)}`).join('') +
          `L${last[0].toFixed(2)},${props.height}Z`
      }
      segment = []
    }
    item.values.forEach((value, index) => {
      if (value === null) flush()
      else segment.push([x(index), y(value)])
    })
    flush()
    return { key: item.key, color: item.color, path, area }
  })
)
</script>
