FROM rust:1.75-slim-bullseye as builder

WORKDIR /app
COPY . .
RUN cargo build --release

FROM debian:bullseye-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/target/release/optimized-server /usr/local/bin/app

RUN useradd -m -U -s /bin/false app
RUN mkdir -p /data && chown app:app /data

USER app

ENV RUSTFLAGS="-C target-cpu=native -C opt-level=3"
ENV RUST_MIN_STACK=8388608

EXPOSE 8080

CMD ["/usr/local/bin/app"]
