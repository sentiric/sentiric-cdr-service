# 📊 Sentiric CDR Service - Görev Listesi (v2.0 - Dayanıklılık ve Bütünlük)

Bu belge, cdr-service'in geliştirme yol haritasını, tamamlanan görevleri ve mevcut öncelikleri tanımlar.

---

### **FAZ 1: Temel Olay Kaydı (Tamamlandı)**

-   [x] **Görev ID: CDR-CORE-01 - Olay Tüketimi**
-   [x] **Görev ID: CDR-CORE-02 - Ham Olay Kaydı**
-   [x] **Görev ID: CDR-CORE-03 - Temel CDR Oluşturma**
-   [x] **Görev ID: CDR-005 - Çağrı Kaydı URL'ini Saklama**

---

### **FAZ 2: Dayanıklılık ve Veri Bütünlüğü (Mevcut Odak)**

**Amaç:** Servisin başlatılmasını daha dayanıklı hale getirmek, olay sırasından kaynaklanabilecek veri kaybını önlemek ve kod tabanını standartlara uygun, temiz bir hale getirmek.

-   **Görev ID: CDR-BUG-02 - Olay Sırası Yarış Durumunu (Race Condition) Çözme (KRİTİK)**
    -   **Durum:** ⬜ **Yapılacak (Öncelik 1)**
    -   **Problem Tanımı:** Mevcut mantık, `call.started` olayının her zaman `user.identified.for_call`'dan önce geleceğini varsaymaktadır. Olayların ters sırada gelmesi durumunda kullanıcı/tenant bilgisi kalıcı olarak kaybolmaktadır.
    -   **Çözüm Stratejisi:** Veritabanı yazma işlemleri "UPSERT" (INSERT ... ON CONFLICT DO UPDATE) mantığına geçirilecektir. `handleCallStarted` ve `handleUserIdentified` fonksiyonları, `calls` tablosuna kayıt eklerken veya güncellerken, kaydın önceden var olup olmamasından etkilenmeyecek şekilde yeniden yazılacaktır. Bu, olay sırasından bağımsız olarak veri bütünlüğünü garanti altına alacaktır.

-   **Görev ID: CDR-REFACTOR-01 - Dayanıklı Başlatma ve Graceful Shutdown**
    -   **Durum:** ⬜ **Yapılacak (Öncelik 2)**
    -   **Problem Tanımı:** Servis, başlangıçta bağımlılıkları (Postgres, RabbitMQ) hazır değilse `log.Fatal` ile çökmektedir. Bu, dağıtık ortamlarda kırılgan bir davranıştır.
    -   **Çözüm Stratejisi:** `agent-service`'te uygulanan dayanıklı başlatma mimarisi buraya da uygulanacaktır. `main.go` ve bağlantı fonksiyonları, servisin hemen başlayıp arka planda periyodik olarak bağlantı denemeleri yapacağı ve `CTRL+C` ile her an kontrollü bir şekilde kapatılabileceği şekilde yeniden yapılandırılacaktır.

-   **Görev ID: CDR-IMPRV-01 - Dockerfile Güvenlik ve Standardizasyonu**
    -   **Durum:** ⬜ **Yapılacak**
    -   **Açıklama:** `Dockerfile`, root kullanıcısıyla çalışmakta ve platformdaki diğer Go servislerinden farklı olarak `alpine` tabanını kullanmaktadır.
    -   **Kabul Kriterleri:**
        -   [ ] `Dockerfile` tabanı, tutarlılık için `debian:bookworm-slim` olarak güncellenmelidir.
        -   [ ] Güvenlik en iyi uygulamalarına uymak için, imaj içinde root olmayan bir `appuser` oluşturulmalı ve uygulama bu kullanıcı ile çalıştırılmalıdır.

-   **Görev ID: CDR-CLEANUP-01 - Gereksiz Kodların Temizlenmesi**
    -   **Durum:** ⬜ **Yapılacak**
    -   **Açıklama:** `internal/database/postgres.go` dosyasında `cdr-service`'in sorumluluk alanına girmeyen `GetAnnouncementPathFromDB` ve `GetTemplateFromDB` fonksiyonları bulunmaktadır.
    -   **Kabul Kriterleri:**
        -   [ ] Bu iki fonksiyon ve bunlarla ilgili olası testler kod tabanından tamamen kaldırılmalıdır.

-   **Görev ID: CDR-IMPRV-03 - Log Zaman Damgasını Standardize Etme**
    -   **Durum:** ⬜ **Yapılacak**
    -   **Açıklama:** Loglardaki zaman damgaları, platform standardı olan UTC ve RFC3339 formatında değildir.
    -   **Kabul Kriterleri:**
        -   [ ] `internal/logger/logger.go` dosyası, `agent-service`'teki standartlaştırılmış versiyon ile güncellenmelidir.

-   **Görev ID: CDR-BUG-01 - Eksik Kullanıcı/Tenant Verisi Sorunu (Güncellendi)**
    -   **Durum:** 🟧 **Bloklandı (AGENT-BUG-04 bekleniyor, CDR-BUG-02 ile çözülecek)**
    -   **Açıklama:** Bu görevin asıl nedeni `agent-service`'in olay yayınlamamasıdır. Ancak, `CDR-BUG-02` görevi tamamlandığında, `cdr-service` olayların sırasından etkilenmeyeceği için bu sorun da temelden çözülmüş olacaktır. Bu görev, `CDR-BUG-02`'nin doğrulaması olarak takip edilecektir.