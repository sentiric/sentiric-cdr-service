# 📊 Sentiric CDR Service (Call Detail Record)

[![Status](https://img.shields.io/badge/status-active-success.svg)]()
[![Language](https://img.shields.io/badge/language-Go-blue.svg)]()
[![Protocol](https://img.shields.io/badge/protocol-RabbitMQ-orange.svg)]()

**Sentiric CDR Service**, Sentiric platformundaki tüm çağrı aktivitelerinin ve yaşam döngüsü olaylarının detaylı kayıtlarını toplar, işler ve faturalandırma, analiz ve raporlama için kalıcı olarak saklar.

Bu servis, platformun "kara kutusu" ve hafızasıdır.

## 🎯 Temel Sorumluluklar

*   **Olay Tüketimi:** `RabbitMQ`'daki `sentiric_events` exchange'ini dinleyerek `call.started` ve `call.ended` gibi tüm çağrı yaşam döngüsü olaylarını tüketir.
*   **Veri Zenginleştirme:** Gelen olaydaki arayan numarası gibi bilgileri kullanarak `user-service`'e gRPC ile danışır ve çağrıyı ilgili kullanıcı/kiracı (tenant) ile ilişkilendirir.
*   **Ham Olay Kaydı:** Gelen her olayın ham (raw) JSON verisini, denetim (audit) ve detaylı analiz için `call_events` tablosuna kaydeder.
*   **Özet Kayıt Oluşturma (CDR):** `call.started` ve `call.ended` olaylarını birleştirerek, raporlama için optimize edilmiş, özet bir çağrı kaydını (`calls` tablosu) oluşturur ve günceller.

## 🛠️ Teknoloji Yığını

*   **Dil:** Go
*   **Asenkron İletişim:** RabbitMQ (`amqp091-go` kütüphanesi)
*   **Veritabanı Erişimi:** PostgreSQL (`pgx` kütüphanesi)
*   **Servisler Arası İletişim:** `user-service`'e gRPC ile.
*   **Gözlemlenebilirlik:** Prometheus metrikleri ve `zerolog` ile yapılandırılmış loglama.

## 🔌 API Etkileşimleri

Bu servis birincil olarak bir **tüketicidir (consumer)**.

*   **Gelen (Tüketici):**
    *   `RabbitMQ`: `sentiric_events` exchange'inden olayları alır.
*   **Giden (İstemci):**
    *   `sentiric-user-service` (gRPC): Arayan numarasını kullanıcı profiliyle eşleştirmek için.
    *   `PostgreSQL`: `call_events` ve `calls` tablolarına veri yazmak için.

## 🚀 Yerel Geliştirme

1.  **Bağımlılıkları Yükleyin:** `go mod tidy`
2.  **Ortam Değişkenlerini Ayarlayın:** `.env.docker` dosyasını `.env` olarak kopyalayın. Platformun diğer tüm servisleri Docker üzerinde çalışıyorsa, adresler doğru olacaktır.
3.  **Servisi Çalıştırın:** `go run ./cmd/cdr-service`

## 🤝 Katkıda Bulunma

Katkılarınızı bekliyoruz! Lütfen projenin ana [Sentiric Governance](https://github.com/sentiric/sentiric-governance) reposundaki kodlama standartlarına ve katkıda bulunma rehberine göz atın.


---
## 🏛️ Anayasal Konum

Bu servis, [Sentiric Anayasası'nın (v11.0)](https://github.com/sentiric/sentiric-governance/blob/main/docs/blueprint/Architecture-Overview.md) **Zeka & Orkestrasyon Katmanı**'nda yer alan merkezi bir bileşendir.