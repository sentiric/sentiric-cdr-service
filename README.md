# 📊 Sentiric CDR Service (Call Detail Record)

[![Status](https://img.shields.io/badge/status-active-success.svg)]()
[![Language](https://img.shields.io/badge/language-Go-blue.svg)]()
[![Protocol](https://img.shields.io/badge/protocol-RabbitMQ-orange.svg)]()

**Sentiric CDR Service**, Sentiric platformundaki tüm çağrı aktivitelerinin ve yaşam döngüsü olaylarının detaylı kayıtlarını toplar, işler ve faturalandırma, analiz ve raporlama için kalıcı olarak saklar.

Bu servis, platformun "kara kutusu" ve hafızasıdır. Asenkron olayları dinleyerek çalışır ve olayların geliş sırasından etkilenmeyecek şekilde dayanıklı bir veri işleme mantığına sahiptir.

## 🎯 Temel Sorumluluklar

*   **Olay Tüketimi:** `RabbitMQ`'daki `sentiric_events` exchange'ini dinleyerek `call.started`, `call.ended`, `user.identified.for_call` gibi tüm çağrı yaşam döngüsü olaylarını tüketir.
*   **Veri Zenginleştirme:** Gelen olaylardaki bilgileri (kullanıcı, tenant, çağrı başlangıç/bitiş zamanları) birleştirerek zengin bir çağrı kaydı oluşturur.
*   **Ham Olay Kaydı:** Gelen her olayın ham (raw) JSON verisini, denetim (audit) ve detaylı analiz için `call_events` tablosuna kaydeder.
*   **Özet Kayıt Oluşturma (CDR):** Farklı olaylardan gelen bilgileri `calls` tablosundaki tek bir özet kayıtta birleştirmek için **UPSERT (INSERT ... ON CONFLICT DO UPDATE)** mantığını kullanır.

## 🛠️ Teknoloji Yığını

*   **Dil:** Go
*   **Asenkron İletişim:** RabbitMQ (`amqp091-go` kütüphanesi)
*   **Veritabanı Erişimi:** PostgreSQL (`pgx` kütüphanesi)
*   **Gözlemlenebilirlik:** Prometheus metrikleri ve `zerolog` ile standartlaştırılmış (UTC, RFC3339) yapılandırılmış loglama.

## 🔌 API Etkileşimleri

Bu servis birincil olarak bir **tüketicidir (consumer)** ve dışarıya doğrudan bir API sunmaz.

*   **Gelen (Tüketici):**
    *   `RabbitMQ`: `sentiric_events` exchange'inden tüm olayları alır.
*   **Giden (İstemci):**
    *   `PostgreSQL`: `call_events` ve `calls` tablolarına veri yazmak için.
    *   *Not: Artık `user-service`'e doğrudan bir gRPC bağımlılığı yoktur. Kullanıcı bilgisi, `user.identified.for_call` olayı üzerinden asenkron olarak alınır.*

## 🚀 Yerel Geliştirme

1.  **Bağımlılıkları Yükleyin:** `go mod tidy`
2.  **Ortam Değişkenlerini Ayarlayın:** `.env.docker` dosyasını `.env` olarak kopyalayın.
3.  **Servisi Çalıştırın:** `go run cmd/cdr-service`

## 🤝 Katkıda Bulunma

Katkılarınızı bekliyoruz! Lütfen projenin ana [Sentiric Governance](https://github.com/sentiric/sentiric-governance) reposundaki kodlama standartlarına ve katkıda bulunma rehberine göz atın.

---
## 🏛️ Anayasal Konum

Bu servis, [Sentiric Anayasası'nın (v11.0)](https://github.com/sentiric/sentiric-governance/blob/main/docs/blueprint/Architecture-Overview.md) **Veri & Raporlama Katmanı**'nda yer alan temel bir bileşendir.

---