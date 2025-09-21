# ========== DOSYA: sentiric-cdr-service/TASKS.md (TAM VE GÜNCEL İÇERİK) ==========
# 📊 Sentiric CDR Service - Görev Listesi (v2.2 - Maliyet ve Analiz Odaklı)

Bu belge, cdr-service'in geliştirme yol haritasını, tamamlanan görevleri ve mevcut öncelikleri tanımlar.

---

### **Gelecek Vizyonu: Detaylı Analiz ve Raporlama**

**Amaç:** Servisin yeteneklerini, sadece çağrı kaydı tutmaktan öte, platformun iş ve operasyonel verimliliğini ölçecek şekilde genişletmek.

-   **Görev ID: CDR-FEAT-01 - Maliyet Hesaplama Entegrasyonu**
    -   **Durum:** ⬜ **Planlandı (Öncelik 1)**
    -   **Problem:** Her bir çağrının platforma olan maliyetini (AI servisleri, telefoni vb.) bilmiyoruz. Bu, kârlılık analizi ve fiyatlandırma için kritik bir eksikliktir.
    -   **Açıklama:** Bu görev, platform genelinde bir çaba gerektirir:
        1.  **`sentiric-config`:** `postgres-init/01_core_schema.sql` dosyasındaki `calls` tablosuna `answer_time TIMESTAMPTZ` sütunu eklenmelidir.
        2.  **`sentiric-sip-signaling-service`:** `ACK` alındığında `call.answered` olayını `timestamp` ile birlikte yayınlamalıdır (Zaten yapılıyor).
        3.  **`sentiric-agent-service`:** LLM ve TTS servislerini her çağırdığında, kullanılan token/karakter sayısı gibi bilgileri içeren yeni `cost.usage.reported` olayları yayınlamalıdır.
        4.  **`cdr-service`:** `call.answered` olayını dinleyerek `calls` tablosundaki `answer_time`'ı doldurmalıdır. Ayrıca `cost.usage.reported` olaylarını dinleyerek `calls` tablosundaki `cost` sütununu güncellemelidir.
    -   **Kabul Kriterleri:**
        -   [ ] `calls` tablosunda `answer_time` alanı dolu.
        -   [ ] Bir çağrı tamamlandığında, `calls` tablosundaki `cost` alanında o çağrının yaklaşık maliyetini yansıtan bir değer bulunuyor.
        Not: Bu task için ADR-006: Evrensel Değer ve Maliyet Analiz (VCA) Motoru Mimarisi Kararlarınız inceleyiniz.

-   **Görev ID: CDR-FEAT-02 - Detaylı AI Etkileşim Loglaması**
    -   **Durum:** ⬜ **Planlandı**
    -   **Açıklama:** `agent-service` tarafından yayınlanacak yeni olayları (`dialog.turn.completed` gibi) dinleyerek, her bir diyalog adımında kullanıcının ne söylediğini, AI'ın ne cevap verdiğini ve RAG sürecinde hangi bilgilerin kullanıldığını `call_dialog_events` adında yeni bir tabloya kaydetmek. Bu, konuşma analizi için paha biçilmez bir veri sağlayacaktır.