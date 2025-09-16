# 📊 Sentiric CDR Service - Görev Listesi (v2.1 - Stabilite ve Bütünlük)

Bu belge, cdr-service'in geliştirme yol haritasını, tamamlanan görevleri ve mevcut öncelikleri tanımlar.

---


### **Gelecek Vizyonu**

**Amaç:** Servisin yeteneklerini, daha detaylı analiz ve raporlama ihtiyaçlarını karşılayacak şekilde genişletmek.

-   **Görev ID: CDR-FEAT-01 - Detaylı AI Etkileşim Loglaması**
    -   **Durum:** ⬜ **Planlandı**
    -   **Açıklama:** `agent-service` tarafından yayınlanacak yeni olayları (`dialog.turn.completed` gibi) dinleyerek, her bir diyalog adımında kullanıcının ne söylediğini, AI'ın ne cevap verdiğini ve RAG sürecinde hangi bilgilerin kullanıldığını `call_dialog_events` adında yeni bir tabloya kaydetmek. Bu, konuşma analizi için paha biçilmez bir veri sağlayacaktır.

-   **Görev ID: CDR-FEAT-02 - Maliyet Hesaplama**
    -   **Durum:** ⬜ **Planlandı**
    -   **Açıklama:** Her çağrı sonunda, kullanılan STT, TTS ve LLM servislerinin süre/token bilgilerini içeren olayları (`cost.usage.reported` gibi) dinleyerek, `calls` tablosuna her bir çağrının yaklaşık maliyetini eklemek.