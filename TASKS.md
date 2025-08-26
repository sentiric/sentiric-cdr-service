# 📊 Sentiric CDR Service - Görev Listesi

Bu belge, `cdr-service`'in geliştirme yol haritasını ve önceliklerini tanımlar.

---

### Faz 1: Temel Kayıt ve Raporlama (Mevcut Durum)

Bu faz, servisin temel çağrı olaylarını kaydedip özet bir CDR oluşturabilmesini hedefler.

-   [x] **RabbitMQ Tüketicisi:** `sentiric_events` exchange'inden tüm olayları dinleme.
-   [x] **Ham Olay Kaydı:** Gelen her olayı `call_events` tablosuna yazma.
-   [x] **Özet CDR Oluşturma:** `call.started` olayında `calls` tablosuna yeni bir kayıt ekleme.
-   [x] **Özet CDR Güncelleme:** `call.ended` olayında ilgili kaydı bulup `end_time` ve `duration` alanlarını güncelleme.
-   [x] **Kullanıcı İlişkilendirme:** `call.started` olayındaki arayan numarasını kullanarak `user-service`'e danışma ve `user_id` ile `tenant_id`'yi `calls` tablosuna kaydetme.

---

### **FAZ 2: Platformun Yönetilebilir Hale Getirilmesi**

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