FROM python:3.12-slim

RUN pip install --no-cache-dir "huggingface_hub[cli,hf_transfer,hf_xet]==0.30.2"

# hf_transfer is a Rust-backed download accelerator
ENV HF_HUB_ENABLE_HF_TRANSFER=1

# The host bind-mounts a local folder here for downloaded files
WORKDIR /download

ENTRYPOINT ["huggingface-cli"]
