import { apiClient } from '@/api/client'

export interface AnnouncementImageUploadResponse {
  url: string
  filename: string
  content_type: string
  size: number
}

/**
 * Uploads an image pasted into the announcement Markdown editor.
 *
 * The backend stores immutable image files and returns a URL that can be embedded
 * directly in Markdown as `![alt](url)`.
 */
export async function uploadAnnouncementImage(file: File): Promise<AnnouncementImageUploadResponse> {
  const body = new FormData()
  body.set('image', file)

  const { data } = await apiClient.post<AnnouncementImageUploadResponse>(
    '/admin/custom/announcements/images',
    body,
    {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    },
  )
  return data
}
