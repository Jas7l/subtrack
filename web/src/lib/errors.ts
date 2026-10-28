import { ApiError } from '../api/client'

// Known server descriptions (English, without the "service: " prefix) →
// Russian text. Some carry details after the known part ("department is not
// empty: it still has 3 active users…", "unknown currency XYZ"): they match
// by that prefix. Unknown ones fall back to a text by status.
const known: Record<string, string> = {
  'invalid email or password': 'Неверный email или пароль',
  'user is blocked': 'Учётная запись заблокирована',
  'authentication required': 'Нужно войти в систему',
  'insufficient permissions': 'Недостаточно прав',
  'current password is incorrect': 'Текущий пароль указан неверно',
  'new password must differ from the current one': 'Новый пароль должен отличаться от текущего',
  'email is already taken': 'Этот email уже занят',
  'you cannot block yourself or remove your own admin role':
    'Нельзя заблокировать себя или снять с себя роль администратора',
  'the last active admin cannot be blocked or lose the admin role':
    'Нельзя заблокировать или понизить последнего активного администратора',
  'department name is already taken': 'Отдел с таким названием уже есть',
  'department is not empty': 'В отделе есть сотрудники или подписки — удалить нельзя',
  'department does not exist': 'Такого отдела нет',
  'head must be an active user with the manager role':
    'Начальником может быть только активный пользователь с ролью «Начальник отдела»',
  'this user already heads another department': 'Этот человек уже руководит другим отделом',
  'no free seats left': 'Свободных мест нет',
  'seats_total cannot be lower than the number of seats in use; revoke licences first':
    'Мест не может быть меньше, чем выдано лицензий: сначала отзовите лишние',
  'the user already holds a licence of this subscription': 'У пользователя уже есть эта лицензия',
  'the licence is already revoked': 'Лицензия уже отозвана',
  'the licence can go only to an active user (a manager: of their own department)':
    'Лицензию можно выдать только активному сотруднику (начальнику — только своего отдела)',
  'the subscription is cancelled or expired': 'Подписка отменена или истекла',
  'unknown subscription': 'Такой подписки нет',
  'unknown currency': 'Неизвестная валюта',
  'a rejection needs a comment': 'При отклонении нужен комментарий',
  'the request waits for the department head': 'Заявка ждёт решения начальника отдела',
  'the request already waits for the admin': 'Заявка уже ждёт решения администратора',
  'the request is already decided or cancelled': 'По заявке уже принято решение или она отменена',
  'the subscription already has a payment on that date': 'На эту дату платёж уже есть',
  'the transaction cannot move to that status': 'Платёж нельзя перевести в этот статус',
  'a currency with this code already exists': 'Валюта с таким кодом уже есть',
  'RUB is the reporting currency: its rate is fixed at 1 and it cannot be deleted':
    'Рубль — основная валюта: его курс всегда 1, удалить его нельзя',
  'the currency is used by subscriptions, requests or payments and cannot be deleted':
    'Валюта используется в подписках, заявках или платежах — удалить нельзя',
  'bad period': 'Неверный период',
  'days must be between 0 and 365': 'Число дней — от 0 до 365',
  'expiring_days and payment_days are both required': 'Укажите оба числа дней',
  'code must be three Latin letters, like USD': 'Код — три латинские буквы, например USD',
  'rate_to_rub must be above 0 with at most 6 decimals, like 92.5':
    'Курс — больше нуля, не больше 6 знаков после запятой',
  'name must not be empty': 'Укажите название',
  'missing required fields': 'Заполните обязательные поля',
  'amount must be a non-negative amount with at most 2 decimals, like 1500.00':
    'Сумма — не меньше нуля, не больше 2 знаков после запятой',
  'estimated_cost must be a non-negative amount with at most 2 decimals, like 1500.00':
    'Стоимость — не меньше нуля, не больше 2 знаков после запятой',
  'payment_date is required': 'Укажите дату платежа',
  'subscription_id is required': 'Выберите подписку',
  'unknown subscription or currency': 'Такой подписки или валюты нет',
  'insufficient permissions: only the head of the department reviews its requests':
    'Решать по заявкам отдела может только его начальник',
  'insufficient permissions: you cannot decide your own request': 'Нельзя решать по своей заявке',
  'insufficient permissions: you are in no department': 'Вы не привязаны к отделу',
  'subscription_id and user_id are required': 'Выберите сотрудника',
  'password must be at least 8 characters': 'Пароль — не короче 8 символов',
  'password must be at most 72 bytes': 'Пароль слишком длинный',
  'seats_total is required for a group subscription': 'Укажите число мест',
  'seats_total must be at least 1': 'Мест должно быть не меньше одного',
  'an individual subscription has exactly 1 seat; use type group for more':
    'У индивидуальной подписки ровно одно место: для нескольких выберите «Групповая»',
  'an enterprise subscription has no seat limit; omit seats_total':
    'У корпоративной подписки нет ограничения мест',
  'end_date must not be before start_date': 'Окончание не может быть раньше начала',
  'approve with either subscription_id or subscription, not both or neither':
    'Выберите подписку или создайте новую',
  'at least one field must be provided': 'Вы ничего не изменили',
  'currency not found': 'Валюта не найдена',
  'transaction not found': 'Платёж не найден',
  'department not found': 'Отдел не найден',
  'assignment not found': 'Лицензия не найдена',
  'user not found': 'Пользователь не найден',
  'request not found': 'Заявка не найдена',
  'subscription not found': 'Подписка не найдена',
}

const byStatus: Record<number, string> = {
  0: 'Сервер недоступен. Проверьте, что он запущен',
  400: 'Проверьте введённые данные',
  401: 'Сессия истекла, войдите заново',
  403: 'Недостаточно прав',
  404: 'Не найдено',
  409: 'Действие невозможно в текущем состоянии',
}

function knownText(description: string): string | undefined {
  if (known[description]) return known[description]
  let best = ''
  for (const key of Object.keys(known)) {
    const tail = description.charAt(key.length)
    if (key.length > best.length && description.startsWith(key) && (tail === ':' || tail === ' ')) {
      best = key
    }
  }
  return best ? known[best] : undefined
}

// requestMovedOn: a decision came too late — the request was decided,
// cancelled or moved to the other stage meanwhile (409 «the request …»).
export function requestMovedOn(err: unknown): boolean {
  return err instanceof ApiError && err.status === 409 && err.description.startsWith('the request ')
}

export function errorText(err: unknown): string {
  if (err instanceof ApiError) {
    return (
      knownText(err.description) ?? byStatus[err.status] ?? 'Ошибка сервера, попробуйте позже'
    )
  }
  return 'Что-то пошло не так'
}
