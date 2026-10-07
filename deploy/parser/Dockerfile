FROM python:3.12.13-slim-bookworm@sha256:4766d8b510c428e595d74b9cc5bbb2fae8e26316fffb4adc89908d79aacd58a2
COPY requirements.lock /opt/requirements.lock
RUN pip install --no-cache-dir -r /opt/requirements.lock \
    && pip uninstall -y pip setuptools \
    && mkdir /inputs /work \
    && chmod 0555 /inputs
USER 65532:65532
WORKDIR /work
ENV MPLBACKEND=Agg OPENBLAS_NUM_THREADS=1 OMP_NUM_THREADS=1
ENTRYPOINT ["/usr/local/bin/python", "-I", "-B"]
