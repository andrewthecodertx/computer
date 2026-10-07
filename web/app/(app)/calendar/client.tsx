'use client'

import { useState, useEffect, useCallback, useMemo } from 'react'
import { Button } from '@/components/ui/button'
import { BookmarkCard } from '@/components/bookmark-card'
import { BookmarkDetailSheet } from '@/components/bookmark-detail-sheet'
import { ChevronLeft, ChevronRight, Share2 } from 'lucide-react'
import { format, startOfMonth, endOfMonth, eachDayOfInterval, isSameDay, isSameMonth, addMonths, subMonths, getDay } from 'date-fns'
import type { Bookmark } from '@/components/app-shell'

const INITIAL_DATE = new Date(2026, 9, 1)

export function CalendarClient() {
  const [currentMonth, setCurrentMonth] = useState<Date>(INITIAL_DATE)
  const [today, setToday] = useState<Date>(INITIAL_DATE)
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([])
  const [selectedDay, setSelectedDay] = useState<Date | null>(null)
  const [selectedBookmark, setSelectedBookmark] = useState<Bookmark | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const now = new Date()
    setCurrentMonth(now)
    setToday(now)
  }, [])

  const load = useCallback(async () => {
    setLoading(true)
    const res = await fetch('/api/bookmarks')
    if (res.ok) setBookmarks((await res.json()) ?? [])
    setLoading(false)
  }, [])

  useEffect(() => { load() }, [load])

  const days = useMemo(() => {
    const start = startOfMonth(currentMonth)
    const end = endOfMonth(currentMonth)
    return eachDayOfInterval({ start, end })
  }, [currentMonth])

  const startDayOfWeek = getDay(startOfMonth(currentMonth))

  const bookmarksByDay = useMemo(() => {
    const map: Record<string, Bookmark[]> = {}
    for (const b of bookmarks) {
      if (b?.dueDate) {
        const key = format(new Date(b.dueDate), 'yyyy-MM-dd')
        if (!map[key]) map[key] = []
        map[key].push(b)
      }
    }
    return map
  }, [bookmarks])

  const selectedBookmarks = useMemo(() => {
    if (!selectedDay) return []
    const key = format(selectedDay, 'yyyy-MM-dd')
    return bookmarksByDay[key] ?? []
  }, [selectedDay, bookmarksByDay])

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="font-display text-2xl font-bold tracking-tight">Calendar</h1>
          <p className="text-sm text-muted-foreground mt-1">Bookmarks organized by due date</p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="icon-sm" onClick={() => setCurrentMonth(subMonths(currentMonth, 1))}>
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <span className="font-medium text-sm min-w-[140px] text-center">{format(currentMonth, 'MMMM yyyy')}</span>
          <Button variant="outline" size="icon-sm" onClick={() => setCurrentMonth(addMonths(currentMonth, 1))}>
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-7 gap-px bg-border rounded-lg overflow-hidden">
        {['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'].map(d => (
          <div key={d} className="bg-muted/50 px-2 py-2 text-center text-xs font-medium text-muted-foreground">{d}</div>
        ))}
        {Array.from({ length: startDayOfWeek }).map((_, i) => (
          <div key={`empty-${i}`} className="bg-card min-h-[80px]" />
        ))}
        {days.map(day => {
          const key = format(day, 'yyyy-MM-dd')
          const dayBookmarks = bookmarksByDay[key] ?? []
          const isSelected = selectedDay ? isSameDay(day, selectedDay) : false
          const isToday = isSameDay(day, today)

          return (
            <div
              key={key}
              onClick={() => setSelectedDay(day)}
              className={`bg-card min-h-[80px] p-1.5 cursor-pointer transition-colors hover:bg-muted/50 ${
                isSelected ? 'ring-2 ring-primary ring-inset' : ''
              }`}
            >
              <span className={`text-xs font-medium inline-flex h-6 w-6 items-center justify-center rounded-full ${
                isToday ? 'bg-primary text-primary-foreground' : 'text-foreground'
              }`}>
                {format(day, 'd')}
              </span>
              <div className="space-y-0.5 mt-0.5">
                {dayBookmarks.slice(0, 2).map((b: Bookmark) => (
                  <div key={b.id} className="text-[10px] truncate rounded bg-primary/10 text-primary px-1 py-0.5 cursor-pointer" onClick={(e: React.MouseEvent) => { e.stopPropagation(); setSelectedBookmark(b) }}>
                    {b.title ?? 'Untitled'}
                  </div>
                ))}
                {dayBookmarks.length > 2 && (
                  <div className="text-[10px] text-muted-foreground px-1">+{dayBookmarks.length - 2} more</div>
                )}
              </div>
            </div>
          )
        })}
      </div>

      {selectedDay && (
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <h2 className="font-medium">{format(selectedDay, 'EEEE, MMMM d, yyyy')}</h2>
          </div>
          {selectedBookmarks.length === 0 ? (
            <p className="text-sm text-muted-foreground">No bookmarks on this date</p>
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
              {selectedBookmarks.map((b: Bookmark) => (
                <BookmarkCard key={b.id} bookmark={b} compact onClick={() => setSelectedBookmark(b)} />
              ))}
            </div>
          )}
        </div>
      )}

      <BookmarkDetailSheet bookmark={selectedBookmark} onClose={() => setSelectedBookmark(null)} onUpdate={load} />
    </div>
  )
}
