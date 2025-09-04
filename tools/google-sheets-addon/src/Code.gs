/* ====== CONFIG DEFAULTS ====== */
const DEFAULT_PROXY_URL = 'https://llm-proxy.mprlab.com/';
const DEFAULT_MODEL = 'gpt-5-mini';

/* ====== STORAGE ====== */
function getUserSettings_() {
  const p = PropertiesService.getUserProperties();
  return {
    proxyUrl: p.getProperty('LLM_PROXY_URL') || DEFAULT_PROXY_URL,
    apiKeyPresent: !!p.getProperty('LLM_API_KEY'),        // do not send key to UI by default
    model: p.getProperty('LLM_MODEL') || DEFAULT_MODEL
  };
}
function setUserSettings_(data) {
  const p = PropertiesService.getUserProperties();
  if (data.proxyUrl !== undefined) p.setProperty('LLM_PROXY_URL', String(data.proxyUrl).trim());
  if (data.model !== undefined)    p.setProperty('LLM_MODEL', String(data.model).trim());
  if (data.apiKey !== undefined && String(data.apiKey).trim() !== '') {
    p.setProperty('LLM_API_KEY', String(data.apiKey).trim());
  }
  // If user explicitly requests clear
  if (data.clearKey === true) {
    p.deleteProperty('LLM_API_KEY');
  }
}
function getApiKey_() {
  return PropertiesService.getUserProperties().getProperty('LLM_API_KEY') || '';
}

/* ====== WORKSPACE ADD-ON ENTRYPOINT ======
   Appears in right sidebar (home card) for Sheets
*/
function buildAddOnHomePage(e) {
  const s = getUserSettings_();

  const section = CardService.newCardSection()
    .addWidget(CardService.newTextParagraph().setText('<b>LLM Settings</b>'));

  // Proxy URL
  section.addWidget(
    CardService.newTextInput()
      .setFieldName('proxyUrl')
      .setTitle('Proxy URL')
      .setValue(s.proxyUrl)
      .setHint('e.g. https://llm-proxy.mprlab.com/')
  );

  // API key (not prefilled)
  section.addWidget(
    CardService.newTextInput()
      .setFieldName('apiKey')
      .setTitle('API Key')
      .setHint(s.apiKeyPresent ? 'Key is saved (leave blank to keep)' : 'Enter your key')
  );

  // Default model
  section.addWidget(
    CardService.newTextInput()
      .setFieldName('model')
      .setTitle('Default Model')
      .setValue(s.model)
      .setHint('e.g. gpt-5-mini')
  );

  // Save button
  const saveAction = CardService.newAction()
    .setFunctionName('handleSave_');
  section.addWidget(
    CardService.newTextButton()
      .setText('Save')
      .setOnClickAction(saveAction)
      .setTextButtonStyle(CardService.TextButtonStyle.FILLED)
  );

  // Divider
  section.addWidget(CardService.newDivider());

  // Regenerate selection
  const regenAction = CardService.newAction().setFunctionName('handleRegenerate_');
  section.addWidget(
    CardService.newTextButton().setText('Regenerate Selection').setOnClickAction(regenAction)
  );

  // Retry errors in sheet
  const retryAction = CardService.newAction().setFunctionName('handleRetryErrors_');
  section.addWidget(
    CardService.newTextButton().setText('Retry Errors In Sheet').setOnClickAction(retryAction)
  );

  // Grant network access (first-run OAuth)
  const grantAction = CardService.newAction().setFunctionName('handleGrantNetwork_');
  section.addWidget(
    CardService.newTextButton().setText('Grant Network Access').setOnClickAction(grantAction)
  );

  const card = CardService.newCardBuilder().addSection(section).build();
  return card;
}

/* ====== ACTION HANDLERS (UI) ====== */
function handleSave_(e) {
  const inputs = (e && e.commonEventObject && e.commonEventObject.formInputs) || {};
  const proxyUrl = getInput_(inputs, 'proxyUrl');
  const apiKey   = getInput_(inputs, 'apiKey');
  const model    = getInput_(inputs, 'model');

  setUserSettings_({
    proxyUrl: proxyUrl || undefined,
    model: model || undefined,
    apiKey: apiKey || undefined
  });

  const resp = CardService.newActionResponseBuilder()
    .setNotification(CardService.newNotification().setText('Saved'))
    .build();
  return resp;
}

function handleGrantNetwork_() {
  const s = getUserSettings_();
  const probe = s.proxyUrl.replace(/\/?$/, '/');
  try {
    UrlFetchApp.fetch(probe, { method: 'get', muteHttpExceptions: true, followRedirects: true });
    return CardService.newActionResponseBuilder()
      .setNotification(CardService.newNotification().setText('Network access granted.'))
      .build();
  } catch (err) {
    return CardService.newActionResponseBuilder()
      .setNotification(CardService.newNotification().setText('Grant failed: ' + String(err).slice(0, 120)))
      .build();
  }
}

function handleRegenerate_() {
  regenerateSelection_();
  return CardService.newActionResponseBuilder()
    .setNotification(CardService.newNotification().setText('Regenerated selection'))
    .build();
}

function handleRetryErrors_() {
  retryErrorsInSheet_();
  return CardService.newActionResponseBuilder()
    .setNotification(CardService.newNotification().setText('Retried error cells'))
    .build();
}

/* ====== SHEETS OPERATIONS (no UI) ====== */
function regenerateSelection_() {
  const sheet = SpreadsheetApp.getActiveSheet();
  const ranges = (sheet.getSelection() && sheet.getSelection().getActiveRangeList())
    ? sheet.getSelection().getActiveRangeList().getRanges()
    : [];
  ranges.forEach((r) => {
    const formulas = r.getFormulas();
    const height = formulas.length;
    const width = height ? formulas[0].length : 0;
    for (let row = 0; row < height; row++) {
      for (let col = 0; col < width; col++) {
        const f = formulas[row][col];
        if (isLLMFormula_(f)) {
          const bumped = bumpNonceInFormula_(f);
          r.getCell(row + 1, col + 1).setFormula(bumped);
        }
      }
    }
  });
}

function retryErrorsInSheet_() {
  const sheet = SpreadsheetApp.getActiveSheet();
  const rng = sheet.getDataRange();
  const formulas = rng.getFormulas();
  const displays = rng.getDisplayValues();
  const height = formulas.length;
  const width = height ? formulas[0].length : 0;

  for (let r = 0; r < height; r++) {
    for (let c = 0; c < width; c++) {
      const f = formulas[r][c];
      const disp = displays[r][c];
      if (isLLMFormula_(f) && isDisplayError_(disp)) {
        const bumped = bumpNonceInFormula_(f);
        rng.getCell(r + 1, c + 1).setFormula(bumped);
      }
    }
  }
}

/* ====== CUSTOM FUNCTIONS (GET-only) ====== */
/** =LLM(prompt, [systemPrompt]) */
function LLM(prompt, systemPrompt) {
  return coreLLMCall_(prompt, systemPrompt, { model: 'gpt-5-mini' });
}
/** =LLM_WEB(prompt, [systemPrompt]) */
function LLM_WEB(prompt, systemPrompt) {
  return coreLLMCall_(prompt, systemPrompt, { model: 'gpt-5-mini', web_search: 1 });
}
/** =LLM_GPT5(prompt, [systemPrompt]) */
function LLM_GPT5(prompt, systemPrompt) {
  return coreLLMCall_(prompt, systemPrompt, { model: 'gpt-5' });
}

function coreLLMCall_(prompt, systemPrompt, overrides) {
  const started = Date.now();
  const budgetMs = 25000;

  const s = getUserSettings_();
  const apiKey = getApiKey_();
  if (!apiKey) throw new Error('LLM: API key not set. Open the add-on and Save your key.');

  const payload = buildPayload_(prompt, systemPrompt, overrides, s, apiKey);
  const url = buildUrl_(s.proxyUrl, payload);

  const options = { method: 'get', muteHttpExceptions: true, followRedirects: true };

  const maxAttempts = 2;
  let attempt = 0;
  let backoffMs = 1200;

  while (true) {
    attempt++;
    try {
      const res = UrlFetchApp.fetch(url, options);
      const code = res.getResponseCode();
      const text = res.getContentText();
      if (code >= 200 && code < 300) {
        return postprocess_(text, payload.format);
      } else {
        if (attempt >= maxAttempts) {
          throw new Error('LLM HTTP ' + code + ': ' + truncate_(text, 500));
        }
      }
    } catch (e) {
      if (attempt >= maxAttempts) {
        throw new Error('LLM failed after retries: ' + String(e));
      }
    }
    if (Date.now() - started + backoffMs > budgetMs) {
      throw new Error('LLM: time budget exceeded before retry.');
    }
    Utilities.sleep(backoffMs);
    backoffMs *= 2;
  }
}

/* ====== SHARED HELPERS ====== */
function buildPayload_(prompt, systemPrompt, overrides, s, apiKey) {
  const isRange = Array.isArray(prompt) && Array.isArray(prompt[0]);
  const flattened = isRange ? flatten2D_(prompt).join('\n') : toStr_(prompt);
  const sys = systemPrompt ? toStr_(systemPrompt) : '';
  const params = { model: s.model || DEFAULT_MODEL, format: 'text/plain', web_search: 0 };
  Object.assign(params, overrides || {});
  params.nonce = String(Date.now());
  return {
    key: apiKey,
    prompt: flattened,
    system_prompt: sys,
    model: params.model,
    web_search: params.web_search,
    format: params.format,
    nonce: params.nonce
  };
}
function buildUrl_(base, p) {
  const q = [
    ['key', p.key], ['prompt', p.prompt], ['system_prompt', p.system_prompt],
    ['model', p.model], ['web_search', String(p.web_search)], ['format', p.format], ['nonce', p.nonce]
  ].map(([k, v]) => k + '=' + encodeURIComponent(String(v)));
  let url = base || DEFAULT_PROXY_URL;
  if (!url.endsWith('/')) url += '/';
  return url + '?' + q.join('&');
}
function postprocess_(text, format) {
  if (format && format.toLowerCase() === 'text/csv') return Utilities.parseCsv(text);
  return text;
}
function isLLMFormula_(f) {
  if (!f) return false;
  const s = String(f).trim().toUpperCase();
  return s.startsWith('=LLM(') || s.startsWith('=LLM_WEB(') || s.startsWith('=LLM_GPT5(');
}
function isDisplayError_(disp) {
  if (!disp) return false;
  return String(disp).trim().startsWith('#');
}
function bumpNonceInFormula_(formula) {
  if (!formula) return formula;
  const hasNonce = /(?:[?&,]\s*nonce\s*=)/i.test(formula);
  const stamp = String(Date.now());
  if (hasNonce) return formula.replace(/(nonce\s*=\s*)(\d+)/i, '$1' + stamp);
  return formula + ' ';
}
function flatten2D_(arr2d) {
  const out = [];
  for (let r = 0; r < arr2d.length; r++) {
    for (let c = 0; c < arr2d[r].length; c++) {
      const v = arr2d[r][c];
      if (v !== null && v !== undefined && v !== '') out.push(String(v));
    }
  }
  return out;
}
function toStr_(v) {
  if (v === null || v === undefined) return '';
  if (Array.isArray(v)) return flatten2D_(v).join('\n');
  return String(v);
}
function truncate_(txt, maxLen) {
  const s = String(txt || '');
  return s.length > maxLen ? s.slice(0, maxLen) + '…' : s;
}

/* ====== Helper to read CardService form input ====== */
function getInput_(formInputs, name) {
  if (!formInputs || !formInputs[name] || !formInputs[name].stringInputs) return '';
  const arr = formInputs[name].stringInputs.value || [];
  return arr.length ? arr[0] : '';
}
