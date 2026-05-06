# archive-basic

Minimal bu1ld project that exercises imports, typed task rules, archive actions,
configuration cache, and local action cache without requiring external tools.

Run from this directory:

```bash
bu1ld tasks
bu1ld graph package_zip
bu1ld build package_zip
bu1ld build package_zip
```

The second build should report `FROM-CACHE package_zip`.
