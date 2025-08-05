# Sentiric CDR Service (Call Detail Record)

**Description:** Collects, processes, and stores detailed records of all call activities and lifecycle events for billing, analysis, and reporting within the Sentiric platform.

**Core Responsibilities:**
*   Consuming call lifecycle events (e.g., Call-Start, Call-Answered, Call-End, Call-Transfer) from a message queue.
*   Aggregating event data to form comprehensive Call Detail Records (CDRs).
*   Persistently storing CDRs in an optimized database for querying and analysis (e.g., PostgreSQL, ClickHouse, Elasticsearch).
*   Optionally, providing APIs for querying and reporting on stored CDR data.

**Technologies:**
*   Node.js (or Python, Go)
*   Message Queue Client (e.g., KafkaJS for Node.js, confluent-kafka-python for Python)
*   Database connection (e.g., PostgreSQL client, Elasticsearch client).
* we can use TimescaleDB (PostgreSQL extension)	Hypertable partitioning uygulayın / Vector DB / Vector Extension for pgsql

**API Interactions (As a Message Consumer & Optional API Provider):**
*   **Consumes Messages From:** `sentiric-sip-server`, `sentiric-media-service` (via Message Queue).
*   **Optionally Provides API For:** `sentiric-dashboard` (for CDR reporting and queries).

**Local Development:**
1.  Clone this repository: `git clone https://github.com/sentiric/sentiric-cdr-service.git`
2.  Navigate into the directory: `cd sentiric-cdr-service`
3.  Install dependencies: `npm install` (Node.js) or `pip install -r requirements.txt` (Python).
4.  Create a `.env` file from `.env.example` to configure message queue and database connections.
5.  Start the service: `npm start` (Node.js) or `python app.py` (Python).

**Configuration:**
Refer to `config/` directory and `.env.example` for service-specific configurations, including message queue and database connection details.

**Deployment:**
Designed for containerized deployment (e.g., Docker, Kubernetes). Requires a robust persistent database. Refer to `sentiric-infrastructure`.

**Contributing:**
We welcome contributions! Please refer to the [Sentiric Governance](https://github.com/sentiric/sentiric-governance) repository for coding standards and contribution guidelines.

**License:**
This project is licensed under the [License](LICENSE).


```bash
go run ./cmd/cdr-service
```