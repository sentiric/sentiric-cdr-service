# 🧬 VCA & Billing Logic
1. **Cost Calculation:** Telefon görüşmeleri saniye bazlı, LLM istekleri ise token bazlı hesaplanır.
2. **Exact-Time Billing:** `call.answered` ve `call.ended` arasındaki fark milisaniye hassasiyetinde ölçülür.
