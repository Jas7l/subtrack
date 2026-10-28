import { ApiError } from '../api/client'

// Known server descriptions (English, without the "service: " prefix) →
// Russian text. Unknown ones fall back to a text by status.
const known: Record<string, string> = {
  'invalid email or password': 'Неверный email или пароль',
  'user is blocked': 'Учётная запись заблокирована',
  'authentication required': 'Нужно войти в систему',
  'insufficient permissions': 'Недостаточно прав',
  'current password is incorrect': 'Текущий пароль указан неверно',
  'email is already taken': 'Этот email уже занят',
  'you cannot block yourself or remove your own admin role':
    'Нельзя заблокировать себя или снять с себя роль администратора',
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
  'insufficient permissions: only the head of the department reviews its requests':
    'Решать по заявкам отдела может только его начальник',
  'insufficient permissions: you are in no department': 'Вы не привязаны к отделу',
  'subscription_id and user_id are required': 'Выберите сотрудника',
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

export function errorText(err: unknown): string {
  if (err instanceof ApiError) {
    return known[err.description] ?? byStatus[err.status] ?? 'Ошибка сервера, попробуйте позже'
  }
  return 'Что-то пошло не так'
}
