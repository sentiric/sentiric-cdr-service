# 📊 Sentiric CDR Service - Mantık ve Akış Mimarisi

**Belge Amacı:** Bu doküman, `cdr-service`'in platformun **"kara kutusu" ve yasal hafızası** olarak rolünü, nasıl çalıştığını ve diğer servislerle etkileşimini açıklar.

---

## 1. Stratejik Rol: "Tarafsız Gözlemci"

Bu servisin tek görevi, platformda olan biten **her şeyi sessizce dinlemek ve kalıcı olarak kaydetmektir.** Hiçbir iş akışını başlatmaz veya etkilemez.

**Bu servis sayesinde:**
1.  **Raporlama Mümkün Olur:** Faturalandırma, analiz ve yönetici panelleri için gerekli olan tüm çağrı detay kayıtları (Call Detail Records - CDR) oluşturulur.
2.  **Denetim (Audit Trail) Sağlanır:** Bir çağrıda ne olduğu, hangi adımlardan geçtiği ve ne zaman gerçekleştiği gibi sorulara kesin cevaplar verilebilir.
3.  **Sistem Sağlığı Korunur:** CDR kaydı, ana çağrı akışından tamamen ayrı ve asenkron olarak yapıldığı için veritabanında yaşanacak bir yavaşlık, canlı çağrıların performansını etkilemez.

---

## 2. Uçtan Uca Kayıt Akışı

```mermaid
sequenceDiagram
    participant SignalingService as SIP Signaling Service
    participant RabbitMQ
    participant CDRService as CDR Service
    participant UserService as User Service
    participant PostgreSQL

    SignalingService->>RabbitMQ: `call.started` olayını yayınlar
    
    RabbitMQ-->>CDRService: Olayı tüketir
    
    Note right of CDRService: Olayın ham halini `call_events`'e yazar.

    CDRService->>PostgreSQL: INSERT INTO call_events (payload: {...})
    
    Note right of CDRService: Kullanıcıyı bulmak için arayan numarasını alır.

    CDRService->>UserService: FindUserByContact(arayan_numarasi)
    UserService-->>CDRService: User(id, tenant_id)
    
    Note right of CDRService: Özet kaydı `calls` tablosuna oluşturur.

    CDRService->>PostgreSQL: INSERT INTO calls (call_id, user_id, tenant_id, start_time, status='STARTED')



    SignalingService->>RabbitMQ: `call.ended` olayını yayınlar
    RabbitMQ-->>CDRService: Olayı tüketir
    
    Note right of CDRService: İlgili özet kaydını günceller.
    
    CDRService->>PostgreSQL: UPDATE calls SET end_time, duration, status='COMPLETED' WHERE call_id=...
```