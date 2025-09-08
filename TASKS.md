# 📊 Sentiric CDR Service - Görev Listesi (v1.7 - Veri Bütünlüğü)

Bu belge, cdr-service'in geliştirme yol haritasını, tamamlanan görevleri ve mevcut öncelikleri tanımlar.

---

### **FAZ 1: Temel Olay Kaydı (Mevcut Durum)**

**Amaç:** RabbitMQ üzerinden gelen tüm çağrı yaşam döngüsü olaylarını dinleyerek ham veriyi (`call_events`) ve temel çağrı özetini (`calls`) oluşturmak.

-   [x] **Görev ID: CDR-CORE-01 - Olay Tüketimi:** RabbitMQ'daki `sentiric_events` exchange'ini dinler ve tüm olayları alır.
-   [x] **Görev ID: CDR-CORE-02 - Ham Olay Kaydı:** Gelen her olayın ham JSON verisini, denetim için `call_events` tablosuna kaydeder.
-   [x] **Görev ID: CDR-CORE-03 - Temel CDR Oluşturma:** `call.started` ve `call.ended` olaylarını işleyerek `calls` tablosunda bir çağrının başlangıç ve bitiş zamanlarını kaydeder.
-   [x] **Görev ID: CDR-005 - Çağrı Kaydı URL'ini Saklama:** `call.recording.available` olayını işleyerek ilgili çağrı kaydının `recording_url` alanını günceller.

---

### **FAZ 2: Veri Bütünlüğü ve Zenginleştirme (Mevcut Odak)**

**Amaç:** `calls` tablosundaki özet kayıtların, çağrıyla ilgili tüm kritik bilgilerle (kullanıcı, tenant vb.) zenginleştirilmesini ve veri bütünlüğünün sağlanmasını garanti altına almak.

-   **Görev ID: CDR-BUG-01 - Eksik Kullanıcı/Tenant Verisi Sorununu Giderme (YÜKSEK ÖNCELİK)**
    -   **Durum:** 🟧 **Bloklandı (AGENT-BUG-04 bekleniyor)**
    -   **Problem Tanımı:** Canlı testlerde, `calls` tablosundaki kayıtların `user_id`, `tenant_id` ve `contact_id` alanlarının `NULL` olarak kaldığı tespit edilmiştir. Bu, raporlama için kritik bir veri kaybıdır.
    -   **Kök Neden Analizi:** Sorunun kök nedeni, `agent-service`'in kullanıcıyı tanımladıktan sonra `user.identified.for_call` olayını yayınlamamasıdır. `cdr-service`, bu olayı işleyecek `handleUserIdentified` fonksiyonuna sahiptir, ancak olay hiç gelmediği için tetiklenememektedir.
    -   **Çözüm Stratejisi ve Doğrulama:**
        -   Bu görev için `cdr-service`'te bir kod değişikliği beklenmemektedir.
        -   `AGENT-BUG-04` görevi tamamlandıktan sonra, yapılacak bir test aramasında bu servisin `user.identified.for_call` olayını alıp `calls` tablosunu doğru bir şekilde güncellediği **doğrulanmalıdır.**
    -   **Kabul Kriterleri:**
        -   [ ] `AGENT-BUG-04` tamamlandıktan sonraki ilk testte, `cdr-service` loglarında "Kullanıcı kimliği bilgisi alındı, CDR güncelleniyor." mesajı görülmelidir.
        -   [ ] Veritabanındaki `calls` tablosunda ilgili `call_id` için `user_id`, `contact_id` ve `tenant_id` sütunlarının doğru verilerle doldurulduğu doğrulanmalıdır.