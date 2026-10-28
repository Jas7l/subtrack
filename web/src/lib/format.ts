import dayjs from 'dayjs'
import customParseFormat from 'dayjs/plugin/customParseFormat'
import relativeTime from 'dayjs/plugin/relativeTime'

dayjs.extend(customParseFormat)
dayjs.extend(relativeTime)

const currencySymbols: Record<string, string> = { RUB: '₽', USD: '$', EUR: '€', CNY: '¥' }

const wholeFormat = new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 0 })
const centsFormat = new Intl.NumberFormat('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 })

// formatAmount shows a server amount without its currency: «15 000»
// ("15000.00"), or with both kopecks when there are any: «1 500,50»
// ("1500.50"). It only formats: sums are computed by the server, never added
// up here.
export function formatAmount(amount: string): string {
  const n = Number(amount)
  if (!Number.isFinite(n)) return amount
  return Number.isInteger(n) ? wholeFormat.format(n) : centsFormat.format(n)
}

// formatMoney is formatAmount with the currency: «15 000 ₽», «54,99 $».
export function formatMoney(amount: string | null | undefined, currency = 'RUB'): string {
  if (amount === null || amount === undefined || amount === '') return '—'
  return `${formatAmount(amount)} ${currencySymbols[currency] ?? currency}`
}

// Dates come as "YYYY-MM-DD" and are parsed as local dates: new Date() would
// read them as UTC and could shift them by a day.
export const parseDate = (date: string) => dayjs(date, 'YYYY-MM-DD')

export function formatDate(date: string | null | undefined): string {
  return date ? parseDate(date).format('DD.MM.YYYY') : '—'
}

// Moments (created_at and the like) are RFC 3339.
export function formatMoment(moment: string | null | undefined): string {
  return moment ? dayjs(moment).format('DD.MM.YYYY HH:mm') : '—'
}

// formatDay shows only the local date of a moment.
export function formatDay(moment: string | null | undefined): string {
  return moment ? dayjs(moment).format('DD.MM.YYYY') : '—'
}

export function fromNow(moment: string): string {
  return dayjs(moment).fromNow()
}

// daysLeft: how many days remain until the date (negative once it passed).
export function daysLeft(date: string): number {
  return parseDate(date).diff(dayjs().startOf('day'), 'day')
}

// plural picks the Russian form for a count: plural(5, 'день', 'дня', 'дней').
export function plural(n: number, one: string, few: string, many: string): string {
  const m10 = Math.abs(n) % 10
  const m100 = Math.abs(n) % 100
  if (m10 === 1 && m100 !== 11) return one
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return few
  return many
}
