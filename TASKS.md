# 📊 Sentiric CDR Service - Görev Listesi (v1.1 - Veri Tutarlılığı)

Bu belge, `cdr-service`'in geliştirme yol haritasını ve önceliklerini tanımlar.

---

### **FAZ 1: Temel Kayıt ve Raporlama (Mevcut Durum)**

**Amaç:** Servisin temel çağrı olaylarını kaydedip özet bir CDR oluşturabilmesini sağlamak.

-   [x] **Görev ID: CDR-CORE-01 - RabbitMQ Tüketicisi**
    -   **Açıklama:** `sentiric_events` exchange'inden tüm olayları dinleme.
    -   **Durum:** ✅ **Tamamlandı**

-   [x] **Görev ID: CDR-CORE-02 - Ham Olay Kaydı**
    -   **Açıklama:** Gelen her olayı `call_events` tablosuna yazma.
    -   **Durum:** ✅ **Tamamlandı**

-   [x] **Görev ID: CDR-CORE-03 - Özet CDR Oluşturma ve Güncelleme**
    -   **Açıklama:** `call.started` olayında `calls` tablosuna yeni bir kayıt ekleme ve `call.ended` olayında ilgili kaydı `end_time` ve `duration` ile güncelleme.
    -   **Durum:** ✅ **Tamamlandı**

-   [x] **Görev ID: CDR-CORE-04 - Kullanıcı İlişkilendirme**
    -   **Açıklama:** `call.started` olayındaki arayan numarasını kullanarak `user-service`'e danışma ve `user_id` ile `tenant_id`'yi `calls` tablosuna kaydetme.
    -   **Durum:** ✅ **Tamamlandı**

-   [x] **Görev ID: CDR-BUG-01 - Telefon Numarası Normalizasyonu (KRİTİK DÜZELTME)**
    -   **Açıklama:** `user-service`'i sorgulamadan önce, SIP `From` başlığından gelen telefon numarasını standart E.164 (`90...`) formatına çeviren bir mantık eklendi.
    -   **Durum:** ✅ **Tamamlandı**
    -   **Not:** Bu düzeltme, kayıtlı kullanıcıların "misafir" olarak algılanması sorununu çözerek veri tutarlılığını sağlamıştır.

-   [x] **Görev ID: CDR-BUG-01 - Telefon Numarası Normalizasyonu**
    -   **Durum:** ✅ **Tamamlandı** (Ancak `user-service`'e taşındığı için bu servisten kaldırıldı).

---

### **FAZ 2: Platformun Yönetilebilir Hale Getirilmesi (Sıradaki Öncelik)**

**Amaç:** Platform yöneticileri ve kullanıcıları için zengin raporlama ve analiz yetenekleri sunmak.
-   [x] **Görev ID: CDR-004 - Olay Tabanlı CDR Zenginleştirme (KRİTİK DÜZELTME)**
    -   **Açıklama:** `call.started` olayında artık kullanıcı bilgisi aranmıyor. Bunun yerine, `agent-service` tarafından yayınlanan `user.created.for_call` olayı dinlenerek, mevcut `calls` kaydı `user_id` ve `contact_id` ile asenkron olarak güncelleniyor.
    -   **Durum:** ✅ **Tamamlandı**
    -   **Not:** Bu değişiklik, `agent-service` ile `cdr-service` arasındaki yarış durumunu (race condition) tamamen ortadan kaldırır.

-   [ ] **Görev ID: CDR-005 - Çağrı Kaydı URL'ini Saklama**
    -   **Açıklama:** `media-service` tarafından yayınlanacak olan `call.recording.available` olayını dinleyerek, ilgili `calls` kaydının `recording_url` alanını güncelle.
    -   **Durum:** ⬜ Planlandı (MEDIA-004'e bağımlı).
        
-   [ ] **Görev ID: CDR-001 - gRPC Raporlama Endpoint'leri**
    -   **Açıklama:** `dashboard-ui` gibi yönetim araçlarının çağrı geçmişini ve temel istatistikleri sorgulayabilmesi için gRPC endpoint'leri oluştur.
    -   **Kabul Kriterleri:**
        -   [ ] `GetCallsByTenant(tenant_id, page, limit)` RPC'si implemente edilmeli.
        -   [ ] `GetCallDetails(call_id)` RPC'si, bir çağrının tüm ham olaylarını (`call_events`) döndürmeli.
        -   [ ] `GetCallMetrics(tenant_id, time_range)` RPC'si, toplam arama sayısı ve ortalama konuşma süresi gibi temel metrikleri sağlamalı.

-   [ ] **Görev ID: CDR-002 - Diğer Olayları İşleme**
    -   **Açıklama:** `call.answered`, `call.transferred` gibi daha detaylı olayları işleyerek `calls` tablosunu zenginleştir. Bu, bir çağrının ne kadar sürede cevaplandığı gibi metrikleri hesaplamayı sağlar.
    -   **Durum:** ⬜ Planlandı.

### **FAZ 3: Optimizasyon**

-   [ ] **Görev ID: CDR-003 - Veri Arşivleme**
    -   **Açıklama:** Çok eski ham olayları (`call_events`) periyodik olarak daha ucuz bir depolama alanına (örn: S3) arşivleyen ve veritabanından silen bir arka plan görevi oluştur.
    -   **Durum:** ⬜ Planlandı.