// Keep API error strings separate from the text shown in the Turkish UI.
const messages: Record<string, string> = {
  "Invalid password": "Şifre yanlış.",
  "Please sign in": "Lütfen giriş yapın.",
  "Admin login is disabled; configure ADMIN_PASSWORD": "Yönetici girişi henüz yapılandırılmamış.",
  "Too many login attempts. Try again in a minute.": "Çok fazla giriş denemesi yaptınız. Bir dakika sonra tekrar deneyin.",
  "Not found": "İçerik bulunamadı.",
  "Endpoint not found": "İstenen adres bulunamadı.",
  "Object storage unavailable": "Dosya depolama hizmetine şu anda ulaşılamıyor.",
  "Object storage is not configured": "Dosya depolama hizmeti henüz yapılandırılmamış.",
  "Storage is busy; retry shortly": "Depolama hizmeti meşgul. Biraz sonra tekrar deneyin.",
  "Content changed or identifier already exists; reload and retry": "İçerik değişmiş veya bu bağlantı zaten kullanılıyor. Sayfayı yenileyip tekrar deneyin.",
  "Write outcome is uncertain; reload before retrying": "İşlemin tamamlanıp tamamlanmadığı doğrulanamadı. Tekrar denemeden önce sayfayı yenileyin.",
  "Operation failed": "İşlem tamamlanamadı. Lütfen tekrar deneyin.",
  "could not upload photo in object storage": "Fotoğraf depolama hizmetine yüklenemedi.",
  "could not synchronize pending changes in object storage": "Bekleyen değişiklikler depolama hizmetine kaydedilemedi. Lütfen biraz sonra tekrar deneyin.",
  "could not write post in object storage": "Yazı depolama hizmetine kaydedilemedi.",
  "Excerpt must be at most 2000 characters": "Özet en fazla 2000 karakter olabilir.",
  "Provide a title, body, lowercase hyphenated slug, and published value 0 or 1": "Başlık ve içerik girin, yayın durumunu seçin. Kısa bağlantıda küçük Latin harfleri, rakamlar ve aralarında tire kullanın.",
  "color must be bw or color": "Renkli veya siyah beyaz seçin.",
  "year_taken must be between 0 and 9999": "Yıl 0 ile 9999 arasında olmalıdır.",
  "Photo metadata is too long (maximum 1900 encoded bytes)": "Fotoğraf bilgileri çok uzun. Ad ve konum alanlarını kısaltın.",
  "Upload a valid JPEG, PNG, or GIF image": "Geçerli bir JPEG, PNG veya GIF fotoğraf yükleyin.",
  "Upload a JPEG, PNG, or GIF image": "JPEG, PNG veya GIF biçiminde bir fotoğraf yükleyin.",
  "Photo must be at most 25 MiB": "Fotoğraf en fazla 25 MiB olabilir.",
  "photo must be between 1 byte and 25 MiB": "Fotoğraf dosyası boş olmamalı ve 25 MiB boyutunu aşmamalıdır.",
  "Photo file is required": "Bir fotoğraf dosyası seçin.",
  "Expected multipart form with photo and metadata fields": "Fotoğraf ve bilgileri gönderilemedi. Lütfen tekrar deneyin.",
  "Provide exactly one photo file and one metadata JSON field": "Tek bir fotoğraf ve ona ait bilgileri gönderin.",
  "Invalid photo metadata JSON": "Fotoğraf bilgileri okunamadı. Sayfayı yenileyip tekrar deneyin.",
  "Expected one metadata JSON object": "Fotoğraf bilgileri okunamadı. Sayfayı yenileyip tekrar deneyin.",
  "Invalid JSON body (maximum 1 MiB)": "Gönderilen içerik okunamadı veya 1 MiB sınırını aşıyor.",
  "Expected a single JSON object": "Gönderilen içerik okunamadı. Sayfayı yenileyip tekrar deneyin.",
  "Invalid pagination": "Geçersiz sayfa numarası veya sayfa boyutu.",
  "page and limit must be positive integers": "Sayfa numarası ve sayfa boyutu pozitif tam sayı olmalıdır.",
  "Pagination exceeds maximum (limit 100, page 1000000)": "Sayfa boyutu en fazla 100, sayfa numarası en fazla 1000000 olabilir.",
};

export function apiErrorMessage(error: unknown): string {
  return typeof error === "string" && messages[error]
    ? messages[error]
    : "İşlem tamamlanamadı. Lütfen tekrar deneyin.";
}
