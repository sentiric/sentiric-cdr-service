# 📊 Sentiric CDR Service - Görev Listesi (v2.1 - Stabilite ve Bütünlük)

Bu belge, cdr-service'in geliştirme yol haritasını, tamamlanan görevleri ve mevcut öncelikleri tanımlar.

---

### **FAZ 1: Temel Olay Kaydı (Tamamlandı)**

-   [x] **Görev ID: CDR-CORE-01 - Olay Tüketimi**
-   [x] **Görev ID: CDR-CORE-02 - Ham Olay Kaydı**
-   [x] **Görev ID: CDR-CORE-03 - Temel CDR Oluşturma**
-   [x] **Görev ID: CDR-005 - Çağrı Kaydı URL'ini Saklama**

---

### **FAZ 2: Dayanıklılık ve Veri Bütünlüğü (Tamamlandı)**

**Amaç:** Servisin başlatılmasını daha dayanıklı hale getirmek, olay sırasından kaynaklanabilecek veri kaybını önlemek ve kod tabanını standartlara uygun, temiz bir hale getirmek.

-   [x] **Görev ID: CDR-BUG-02 - Olay Sırası Yarış Durumunu (Race Condition) Çözme**
-   [x] **Görev ID: CDR-REFACTOR-01 - Dayanıklı Başlatma ve Graceful Shutdown**
-   [x] **Görev ID: CDR-IMPRV-01 - Dockerfile Güvenlik ve Standardizasyonu**
-   [x] **Görev ID: CDR-CLEANUP-01 - Gereksiz Kodların Temizlenmesi**
-   [x] **Görev ID: CDR-IMPRV-03 - Log Zaman Damgasını Standardize Etme**

---

### **FAZ 2.5: Veri Bütünlüğü Doğrulaması (Mevcut Odak)**

-   **Görev ID: CDR-FIX-01 - `user.identified.for_call` Olayı Eksikliğini Yönetme**
    -   **Durum:** ⬜ **Yapılacak (Öncelik 1 - YÜKSEK)**
    -   **Bağımlılık:** `agent-service`'teki `AGENT-BUG-03`'ün tamamlanması.
    -   **Açıklama:** Canlı test logları, `agent-service`'in `user.identified.for_call` olayını yayınlamadığını doğrulamıştır. Bu nedenle `calls` tablosundaki `user_id`, `contact_id` ve `tenant_id` alanları `null` kalmaktadır. `agent-service`'teki hata düzeltildikten sonra, bu servisin gelen `user.identified.for_call` olayını doğru bir şekilde işlediği ve `calls` tablosunu `UPSERT` mantığıyla güncellediği doğrulanmalıdır. Bu, bir kod değişikliğinden çok bir **doğrulama görevidir.**
    -   **Kabul Kriterleri:**
        -   [ ] `agent-service`'teki düzeltme yapıldıktan sonra, yeni bir test araması yapıldığında, `calls` tablosundaki ilgili kaydın `user_id`, `contact_id` ve `tenant_id` alanlarının doğru verilerle doldurulduğu veritabanından doğrulanmalıdır.
---

### **FAZ 3: Gelecek Vizyonu**

**Amaç:** Servisin yeteneklerini, daha detaylı analiz ve raporlama ihtiyaçlarını karşılayacak şekilde genişletmek.

-   **Görev ID: CDR-FEAT-01 - Detaylı AI Etkileşim Loglaması**
    -   **Durum:** ⬜ **Planlandı**
    -   **Açıklama:** `agent-service` tarafından yayınlanacak yeni olayları (`dialog.turn.completed` gibi) dinleyerek, her bir diyalog adımında kullanıcının ne söylediğini, AI'ın ne cevap verdiğini ve RAG sürecinde hangi bilgilerin kullanıldığını `call_dialog_events` adında yeni bir tabloya kaydetmek. Bu, konuşma analizi için paha biçilmez bir veri sağlayacaktır.

-   **Görev ID: CDR-FEAT-02 - Maliyet Hesaplama**
    -   **Durum:** ⬜ **Planlandı**
    -   **Açıklama:** Her çağrı sonunda, kullanılan STT, TTS ve LLM servislerinin süre/token bilgilerini içeren olayları (`cost.usage.reported` gibi) dinleyerek, `calls` tablosuna her bir çağrının yaklaşık maliyetini eklemek.