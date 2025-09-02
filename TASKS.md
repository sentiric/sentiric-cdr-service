# 📊 Sentiric CDR Service - Görev Listesi (v1.3 - Zenginleştirilmiş Kayıt)

Bu belge, `cdr-service`'in geliştirme yol haritasını ve önceliklerini tanımlar.


# Görev Tanımı: Çağrı Kayıtlarını Dinleme Özelliği Entegrasyonu

-   **Servis:** `cdr-service` (veya ilgili arayüz servisi)
-   **Bağımlılık:** `media-service`'teki `MEDIA-FEAT-02` görevinin tamamlanması.
-   **Amaç:** Kullanıcıların (yöneticiler, kalite ekipleri vb.) web arayüzü üzerinden bir çağrının ses kaydını doğal ve anlaşılır bir şekilde dinlemesini sağlamak.
-   **Mevcut Durum:** Çağrı kayıtları S3'te teknik bir formatta saklanmaktadır. Bu formatın doğrudan oynatılması, kötü bir kullanıcı deneyimi sunar.
-   **Yeni Mimari:**
    1.  `media-service` artık `GetPlayableRecording` adında yeni bir gRPC streaming RPC'si sunmaktadır.
    2.  Bu RPC, S3'teki bir kaydın URI'sini alıp, anlık olarak dönüştürülmüş ve "dinlenebilir" bir ses akışı (stream) döndürür.
-   **Uygulama Adımları:**
    -   [ ] **1. Arayüz (Frontend):** Çağrı detayları sayfasında, ses kaydı mevcutsa bir "Oynat" butonu ve HTML `<audio>` elementi gösterilmelidir.
    -   [ ] **2. Backend (`cdr-service`):**
        -   [ ] Frontend'den gelen "kaydı oynat" isteği için yeni bir HTTP endpoint'i (`/api/calls/{call_id}/recording/play`) oluşturulmalıdır.
        -   [ ] Bu endpoint tetiklendiğinde, `cdr-service` ilgili çağrının `recording_uri`'sini veritabanından okumalıdır.
        -   [ ] `media-service`'in `GetPlayableRecording` RPC'sine bu URI ile bir istek gönderilmelidir.
        -   [ ] `media-service`'ten gelen ses akışı (gRPC stream), doğrudan HTTP yanıtına (HTTP stream) aktarılmalıdır. Bu, ses dosyasının tamamının `cdr-service`'in belleğine yüklenmesini engeller ve verimli bir akış sağlar.
        -   [ ] HTTP yanıtının `Content-Type` başlığı doğru ayarlanmalıdır (örn: `audio/mpeg`).
    -   [ ] **3. Uçtan Uca Akış:** Kullanıcı "Oynat" butonuna bastığında, frontend bu yeni endpoint'e istek yapmalı ve tarayıcı, gelen ses akışını `<audio>` elementi üzerinden sorunsuzca oynatmalıdır.
    
    
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

-   [ ] **Görev ID:** `CDR-BUG-02` / `AGENT-BUG-04`
    -   **Açıklama:** `cdr-service`'in `call.started` olayında kullanıcı bilgisi aramaktan vazgeçmesini sağla. Bunun yerine, `agent-service`'in, bir misafir kullanıcıyı oluşturduktan veya mevcut bir kullanıcıyı bulduktan sonra, `user_id`, `contact_id` ve `tenant_id` içeren yeni bir `user.identified.for_call` olayı yayınlamasını sağla. `cdr-service` bu yeni olayı dinleyerek mevcut `calls` kaydını güncellemeli.
    -   **Kabul Kriterleri:**
        *   [ ] `sentiric-contracts`'e yeni `UserIdentifiedForCallEvent` mesajı eklenmeli.
        *   [ ] `agent-service`, kullanıcıyı bulduktan/oluşturduktan sonra bu olayı yayınlamalı.
        *   [ ] `cdr-service`, bu olayı dinleyip ilgili `calls` satırını `UPDATE` etmeli.
        *   [ ] Test çağrısı sonunda `calls` tablosundaki `user_id`, `contact_id` ve `tenant_id` alanlarının doğru bir şekilde doldurulduğu doğrulanmalıdır.

- [ ] **Görev ID: CDR-006 - Çağrı Maliyetlendirme**
    - **Durum:** ⬜ Planlandı
    - **Bağımlılık:** `CDR-BUG-02` ve `SIG-BUG-01`'in çözülmesine bağlı.
    - **Açıklama:** `calls` tablosuna `cost` (NUMERIC) adında bir sütun ekle. `tenants` tablosuna `cost_per_second` gibi bir alan ekle. `call.ended` olayı işlenirken, çağrının `duration_seconds` ve ilgili `tenant`'ın dakika başına maliyetine göre `cost` alanını hesapla ve kaydet.
    - **Kabul Kriterleri:**
        - [ ] Veritabanı şeması güncellenmeli.
        - [ ] `handleCallEnded` fonksiyonu, `tenant_id` üzerinden maliyet oranını okuyup hesaplama yapmalı.
        - [ ] Test çağrısı sonunda `cost` alanının doğru bir şekilde doldurulduğu doğrulanmalıdır.

-   [ ] **Görev ID: CDR-005 - Çağrı Kaydı URL'ini Saklama (YÜKSEK ÖNCELİK)**
    -   **Durum:** ⬜ **Yapılacak (ACİL)**
    -   **Bağımlılık:** `media-service`'deki `MEDIA-004`'ün tamamlanmasına bağlı.
    -   **Açıklama:** `media-service` tarafından yayınlanacak olan `call.recording.available` olayını dinleyerek, ilgili `calls` kaydının `recording_url` alanını S3 URI'si ile güncellemek.
    -   **Kabul Kriterleri:**
        -   [ ] `cdr-service`'in `event_handler`'ı, `call.recording.available` olayını işleyecek yeni bir `case` içermelidir.
        -   [ ] Bu olay işlendiğinde, PostgreSQL'deki `calls` tablosunda ilgili `call_id`'ye sahip satırın `recording_url` sütununun güncellendiği doğrulanmalıdır.
        
---

### **FAZ 2: Platformun Yönetilebilir Hale Getirilmesi (Sıradaki Öncelik)**

-   [x] **Görev ID: CDR-REFACTOR-01 - Yarış Durumunu Ortadan Kaldırma (KRİTİK)**
    -   **Durum:** ⬜ **Tammalandı**
    -   **Bağımlılık:** `agent-service`'deki `AGENT-BUG-04` görevinin tamamlanmasına bağlı.
    -   **Bulgular:** `calls` tablosundaki `user_id` gibi alanların `(NULL)` kalması, mevcut `call.started` olayında kullanıcı arama mantığının bir yarış durumu (race condition) yarattığını ve etkisiz olduğunu göstermektedir.
    -   **Çözüm Stratejisi:** `cdr-service`, kullanıcı kimliğini senkron olarak bulmaya çalışmaktan vazgeçmeli ve bu bilgiyi `agent-service`'ten asenkron bir olayla almalıdır.
    -   **Kabul Kriterleri:**
        -   [ ] `handleCallStarted` fonksiyonu, artık `user-service`'i çağırmamalıdır. Sadece `call_id` ve `start_time` ile temel bir kayıt oluşturmalıdır.
        -   [ ] `user.identified.for_call` olayını dinleyecek ve bu olaydaki `user_id`, `contact_id`, `tenant_id` bilgileriyle mevcut `calls` kaydını `UPDATE` edecek yeni bir olay işleyici (`handleUserIdentified`) fonksiyonu oluşturulmalıdır.
        -   [ ] Test çağrısı sonunda `calls` tablosundaki ilgili kaydın `user_id`, `contact_id` ve `tenant_id` alanlarının `(NULL)` olmadığı doğrulanmalıdır.
    -   **Tahmini Süre:** ~1 saat (Bağımlılık çözüldükten sonra)
    
**Amaç:** Platform yöneticileri ve kullanıcıları için zengin raporlama ve analiz yetenekleri sunmak.
-   [x] **Görev ID: CDR-004 - Olay Tabanlı CDR Zenginleştirme (KRİTİK DÜZELTME)**
    -   **Açıklama:** `call.started` olayında artık kullanıcı bilgisi aranmıyor. Bunun yerine, `agent-service` tarafından yayınlanan `user.created.for_call` olayı dinlenerek, mevcut `calls` kaydı `user_id` ve `contact_id` ile asenkron olarak güncelleniyor.
    -   **Durum:** ✅ **Tamamlandı**
    -   **Not:** Bu değişiklik, `agent-service` ile `cdr-service` arasındaki yarış durumunu (race condition) tamamen ortadan kaldırır.


-   [ ] **Görev ID: CDR-005 - Çağrı Kaydı URL'ini Saklama (YÜKSEK ÖNCELİK)**
    -   **Durum:** ⬜ Planlandı
    -   **Bağımlılık:** `MEDIA-004`'ün tamamlanmasına bağlı.
    -   **Tahmini Süre:** ~1-2 saat
    -   **Açıklama:** `media-service` tarafından yayınlanacak olan `call.recording.available` olayını dinleyerek, ilgili `calls` kaydının `recording_url` alanını S3 URI'si ile güncellemek.
    -   **Kabul Kriterleri:**
        -   [ ] `cdr-service`'in `event_handler`'ı, `call.recording.available` olayını işleyecek yeni bir case içermelidir.
        -   [ ] Bu olay işlendiğinde, PostgreSQL'deki `calls` tablosunda ilgili `call_id`'ye sahip satırın `recording_url` sütununun güncellendiği doğrulanmalıdır.

-   [ ] **Görev ID: CDR-BUG-02 - Boş Event Type Sorununu Araştırma**
    -   **Durum:** ⬜ Planlandı (Düşük Öncelik)
    -   **Açıklama:** Test loglarında `event_type` alanı boş olan bir olay kaydedildiği görüldü. Bu, muhtemelen `agent-service`'in çökmesinin bir yan etkisidir. Ana hata (`AGENT-BUG-02`) giderildikten sonra bu sorunun devam edip etmediğini gözlemlemek.
    -   **Kabul Kriterleri:**
        -   [ ] Ana diyalog akışı düzeltildikten sonra, `call_events` tablosunda artık `event_type` alanı boş olan kayıtların oluşmadığı doğrulanmalıdır.

-   [ ] **Görev ID: CDR-001 - gRPC Raporlama Endpoint'leri**
    -   **Açıklama:** `dashboard-ui` gibi yönetim araçlarının çağrı geçmişini ve temel istatistikleri sorgulayabilmesi için gRPC endpoint'leri oluştur.
    -   **Kabul Kriterleri:**
        -   [ ] `GetCallsByTenant(tenant_id, page, limit)` RPC'si implemente edilmeli.
        -   [ ] `GetCallDetails(call_id)` RPC'si, bir çağrının tüm ham olaylarını (`call_events`) döndürmeli.
        -   [ ] `GetCallMetrics(tenant_id, time_range)` RPC'si, toplam arama sayısı ve ortalama konuşma süresi gibi temel metrikleri sağlamalı.

-   [ ] **Görev ID: CDR-002 - Diğer Olayları İşleme**
    -   **Açıklama:** `call.answered`, `call.transferred` gibi daha detaylı olayları işleyerek `calls` tablosunu zenginleştir. Bu, bir çağrının ne kadar sürede cevaplandığı gibi metrikleri hesaplamayı sağlar.
    -   **Durum:** ⬜ Planlandı.

-   [ ] **Görev ID: CDR-002 - Zengin Diyalog Olaylarını İşleme (YENİ)**
    -   **Durum:** ⬜ Planlandı
    -   **Bağımlılık:** `AGENT-EVENT-01`'in tamamlanmasına bağlı.
    -   **Tahmini Süre:** ~1 gün
    -   **Açıklama:** `agent-service` tarafından yayınlanacak olan `call.transcription.available` gibi yeni olay türlerini dinleyerek, bu verileri `calls` tablosundaki ilgili kayda eklemek (örn: tam transkripti bir JSONB sütununa yazmak) veya analiz için ayrı tablolara işlemek.
    -   **Kabul Kriterleri:**
        -   [ ] `calls` tablosuna `full_transcript` adında bir `JSONB` sütunu eklenmelidir.
        -   [ ] `cdr-service`, `call.transcription.available` olayını aldığında, olaydaki metni ilgili `call_id`'ye sahip kaydın `full_transcript` sütununa eklemelidir.
        -   [ ] Bir test çağrısı sonunda, veritabanında `full_transcript` sütununun konuşmanın metnini içerdiği doğrulanmalıdır.    

### **FAZ 3: Optimizasyon**

-   [ ] **Görev ID: CDR-006 - Çağrı Maliyetlendirme**
    -   **Durum:** ⬜ Planlandı
    -   **Bağımlılık:** `CDR-REFACTOR-01` ve `SIG-BUG-01`'in çözülmesine bağlı.
    -   **Açıklama:** Platformun ticari değerini artırmak için çağrı başına maliyet hesaplama yeteneği eklemek.
    -   **Kabul Kriterleri:**
        -   [ ] `calls` tablosuna `cost` (NUMERIC) ve `tenants` tablosuna `cost_per_minute` (NUMERIC) sütunları eklenmeli.
        -   [ ] `handleCallEnded` fonksiyonu, `tenant_id` üzerinden maliyet oranını okuyup, `duration_seconds` ile çarparak `cost` alanını hesaplamalı ve kaydetmelidir.
        

-   [ ] **Görev ID: CDR-003 - Veri Arşivleme**
    -   **Açıklama:** Çok eski ham olayları (`call_events`) periyodik olarak daha ucuz bir depolama alanına (örn: S3) arşivleyen ve veritabanından silen bir arka plan görevi oluştur.
    -   **Durum:** ⬜ Planlandı.