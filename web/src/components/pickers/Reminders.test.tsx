import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ClockContext } from '@/hooks/useNow'
import { atLocal } from '@/lib/dates'
import { Reminders } from './Reminders'

const today = '2026-10-26'
const now = atLocal(today, '14:07')

function renderReminders(reminders: string[], onChange = vi.fn()) {
  render(
    <ClockContext.Provider value={{ now, today }}>
      <Reminders layout="stack" reminders={reminders} end="2026-10-29" onChange={onChange} />
    </ClockContext.Provider>,
  )
  return onChange
}

describe('Reminders', () => {
  it('shows a reminder that went off as sent, without an edit button', () => {
    renderReminders([atLocal(today, '11:00').toISOString()])
    expect(screen.getByLabelText('Reminder sent Mon, Oct 26 · 11:00')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Edit reminder/ })).toBeNull()
    expect(screen.getByRole('button', { name: 'Remove reminder Mon, Oct 26 · 11:00' })).toBeInTheDocument()
  })

  it('adds a preset with one click, including the morning of the end date', () => {
    const onChange = renderReminders([])
    fireEvent.click(screen.getByRole('button', { name: 'Add reminder' }))
    expect(screen.getByText('Thu 29 · 09:00')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Tomorrow morning/ }))
    expect(onChange).toHaveBeenCalledWith([atLocal('2026-10-27', '09:00').toISOString()])
  })

  it('rejects a malformed or past time', () => {
    const onChange = renderReminders([])
    fireEvent.click(screen.getByRole('button', { name: 'Add reminder' }))
    const time = screen.getByRole('textbox', { name: 'Time' })

    fireEvent.change(time, { target: { value: '9:30' } })
    expect(screen.getByRole('alert')).toHaveTextContent('Use 24-hour time, like 09:30.')

    fireEvent.click(screen.getByRole('button', { name: /Mon, Oct 26, today/ }))
    fireEvent.change(time, { target: { value: '13:30' } })
    expect(screen.getByRole('alert')).toHaveTextContent("That time has already passed. It's 14:07 now.")

    const submit = screen.getAllByRole('button', { name: 'Add reminder' }).at(-1)!
    expect(submit).toBeDisabled()
    fireEvent.change(time, { target: { value: '15:30' } })
    fireEvent.click(submit)
    expect(onChange).toHaveBeenCalledWith([atLocal(today, '15:30').toISOString()])
  })
})
