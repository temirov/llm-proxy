# LLM for Google Sheets (Workspace Add-on)

## Local setup

```bash
cd tools/google-sheets-addon
npm i
make login
```

## Script IDs

* DEV: set in `.clasp.dev.json`
* PROD: set in `.clasp.prod.json`

Create two Apps Script projects in the Apps Script editor (Empty project) and copy their Script IDs here.

## Push / Deploy

```bash
# dev
make push-dev
make version-dev
make deploy-dev    # then "Test deployments" → Install in Sheets

# prod
make push-prod
make version-prod
make deploy-prod
```

## Files

* `src/Code.gs` — CardService add-on + custom functions.
* `src/appsscript.json` — manifest (scopes + add-on config).

## Notes

* Secrets: stored via Apps Script `PropertiesService` (never in Git).
* The add-on calls your proxy with GET. No HTML/CSS — native CardService UI.

---

## How to get the two Script IDs (once)

1) Go to script.google.com, **New project** → name it `LLM for Sheets (Dev)`.
    - Copy **Script ID** into `.clasp.dev.json`.
2) Create another: `LLM for Sheets (Prod)`.
    - Copy **Script ID** into `.clasp.prod.json`.
