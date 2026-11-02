# Demo assets

`fixtures/photo.jpg` is a **synthetic** JPEG created for the demo. It contains
fabricated EXIF fields (make, model, timestamps, GPS coordinates) and no real
person, device or location data.

Run the end-to-end demo from the repository root:

```sh
make demo
```

The script creates a temporary case, acquires the synthetic files, records a
handover, seals the case, writes reports and verifies everything. Every item
gets an EXIF/document metadata sidecar whose SHA-256 is recorded in the evidence
chain. Nothing is written into this directory.
