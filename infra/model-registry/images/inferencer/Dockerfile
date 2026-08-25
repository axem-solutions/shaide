FROM python:3.12-slim

# Install torch CPU-only first so sentence-transformers does not pull the CUDA variant
RUN pip install --no-cache-dir torch --index-url https://download.pytorch.org/whl/cpu && \
    pip install --no-cache-dir sentence-transformers einops

COPY infer.py /infer.py

ENTRYPOINT ["python3", "/infer.py"]
