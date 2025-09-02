# 📊 Sentiric CDR Service - Görev Listesi (v1.4 - Veri Bütünlüğü)

Bu belge, `cdr-service`'in platformdaki tüm çağrı verilerini eksiksiz ve doğru bir şekilde kaydetmesini sağlamak için gereken görevleri tanımlar.

---

### **FAZ 1: Temel Kayıt ve Raporlama (Tamamlanmış Görevler)**
*   [x] **CDR-CORE-01 - CDR-CORE-04**: Temel olay dinleme, ham kayıt ve özet CDR oluşturma.
*   [x] **CDR-REFACTOR-01**: Yarış Durumunu Ortadan Kaldırma (Asenkron `user.identified` olayına geçiş).

---

### **FAZ 2: Eksik Verileri Tamamlama (Mevcut Odak)**

-   **Görev ID: CDR-FEAT-01 - Doğru Süre ve Durum Hesaplama**
    -   **Durum:** ✅ **Tamamlandı**
    -   **Öncelik:** YÜKSEK
    -   **Stratejik Önem:** Raporlama, faturalandırma ve analiz için en temel metrik olan "konuşma süresinin" doğru hesaplanmasını sağlar.
    -   **Problem Tanımı:** `duration_seconds` alanı, sinyalizasyon süresini gösteriyordu, gerçek konuşma süresini değil. `answer_time` ve `disposition` alanları `NULL` kalıyordu.
    -   **Bağımlılıklar:** `SIG-FEAT-01` (`sip-signaling`'in `call.answered` olayını yayınlaması).
    -   **Kabul Kriterleri:**
        -   [x] `event_handler.go` içine `call.answered` olayı için yeni bir `case` bloğu eklenmelidir.
        -   [x] Bu olay işlendiğinde, `calls` tablosundaki `answer_time` alanı ve `disposition` alanı (`ANSWERED` olarak) güncellenmelidir.
        -   [x] `handleCallEnded` fonksiyonundaki `duration_seconds` hesaplaması, `end_time - answer_time` olacak şekilde değiştirilmelidir.
        -   [x] Test çağrısı sonunda `duration_seconds` alanının, gerçek konuşma süresine yakın bir değer içerdiği doğrulanmalıdır.
    -   **Tahmini Süre:** ~3-4 Saat

-   **Görev ID: CDR-005 - Çağrı Kaydı URL'ini Saklama**
    -   **Durum:** ⬜ **Yapılacak (Bloklandı)**
    -   **Öncelik:** YÜKSEK
    -   **Stratejik Önem:** Kullanıcıların ve yöneticilerin çağrı kayıtlarını dinleyebilmesi için temel bir gerekliliktir. Bu olmadan, kayıtlar S3'te var olsa bile erişilemez durumdadır.
    -   **Problem Tanımı:** Test çağrısı sonunda `calls` tablosundaki `recording_url` alanı `NULL` kalmıştır.
    -   **Bağımlılıklar:** `MEDIA-004` (`media-service`'in `call.recording.available` olayını yayınlaması).
    -   **Kabul Kriterleri:**
        -   [ ] `event_handler.go` içinde `call.recording.available` olayı için yeni bir `case` bloğu eklenmelidir.
        -   [ ] Bu olay işlendiğinde, PostgreSQL'deki `calls` tablosunda ilgili `call_id`'ye sahip satırın `recording_url` sütunu, olaydaki S3 URI'si ile güncellenmelidir.
    -   **Tahmini Süre:** ~2 Saat

---

### **FAZ 3: Gelişmiş Özellikler**
-   [ ] **Görev ID: CDR-006 - Çağrı Maliyetlendirme**
    -   **Durum:** ⬜ **Planlandı**
    -   **Öncelik:** ORTA
    -   **Bağımlılıklar:** `CDR-FEAT-01`'in tamamlanması.
    -   **Tahmini Süre:** ~1 Gün

-   [ ] **Görev ID: CDR-FEAT-02 - Çağrı Kayıtlarını Dinleme Özelliği**
    -   **Durum:** ⬜ **Planlandı**
    -   **Öncelik:** ORTA
    -   **Açıklama:** Yöneticilerin web arayüzünden bir çağrının ses kaydını dinlemesini sağlayacak bir backend endpoint'i oluşturmak.
    -   **Bağımlılık:** `MEDIA-FEAT-02` (`media-service`'in perde düzeltmeli ses akışı sunması).