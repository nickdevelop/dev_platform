// API client for Note Management.
import { get, mutate } from './client'

export interface Note {
  id: number
  projectId: number
  title: string
  content: string
  createdAt: string
  updatedAt: string
}

export interface CreateNotePayload {
  projectId?: number
  title?: string
  content?: string
}

export interface UpdateNotePayload {
  projectId?: number
  title?: string
  content?: string
}

interface NoteListResponse {
  notes: Note[]
}

interface NoteDetailResponse {
  note: Note
}

export async function listNotes(keyword?: string, projectId?: number): Promise<Note[]> {
  const params = new URLSearchParams()
  if (keyword) params.append('q', keyword)
  if (projectId !== undefined) params.append('projectId', String(projectId))
  const qs = params.toString() ? `?${params.toString()}` : ''
  const res = await get<NoteListResponse>(`/notes${qs}`)
  return res.notes
}

export async function getNote(id: number): Promise<Note> {
  const res = await get<NoteDetailResponse>(`/notes/${id}`)
  return res.note
}

export async function createNote(payload: CreateNotePayload): Promise<Note> {
  const res = await mutate<NoteDetailResponse>('POST', '/notes', payload)
  return res.note
}

export async function updateNote(id: number, payload: UpdateNotePayload): Promise<Note> {
  const res = await mutate<NoteDetailResponse>('PUT', `/notes/${id}`, payload)
  return res.note
}

export async function deleteNote(id: number): Promise<void> {
  await mutate<{ ok: boolean }>('DELETE', `/notes/${id}`)
}
