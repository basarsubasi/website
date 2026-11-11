import { apiErrorMessage } from "./api-errors";

export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
  }
}
export async function api<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const headers = new Headers(options.headers);
  if (!(options.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }
  const response = await fetch("/api" + path, {
    ...options,
    headers,
  });
  if (response.status === 204) return undefined as T;
  const data = await response.json();
  if (!response.ok)
    throw new ApiError(apiErrorMessage(data.error), response.status);
  return data as T;
}
export const send = (method: string, body?: unknown): RequestInit => ({
  method,
  ...(body === undefined ? {} : { body: JSON.stringify(body) }),
});
export const message = (error: unknown): string =>
  error instanceof TypeError
    ? "Sunucuya ulaşılamadı. Bağlantınızı kontrol edip tekrar deneyin."
    : error instanceof SyntaxError
      ? "Sunucudan geçersiz bir yanıt alındı. Lütfen tekrar deneyin."
      : error instanceof Error
        ? error.message
        : "Bir hata oluştu. Lütfen tekrar deneyin.";
