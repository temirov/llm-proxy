/* ====== CONFIG DEFAULTS ====== */
const DEFAULT_PROXY_URL = 'https://llm-proxy.mprlab.com/';
const DEFAULT_MODEL = 'gpt-5-mini';

/* ====== USER SETTINGS (per-user) ====== */
function getUserSettings_() {
  const p = PropertiesService.getUserProperties();
  return {
    proxyUrl: p.getProperty('LLM_PROXY_URL') || DEFAULT_PROXY_URL,
    apiKeyPresent: !!p.getProperty('LLM_API_KEY'),
    model: p.getProperty('LLM_MODEL') || DEFAULT_MODEL
  };
}
function setUserSettings_(data) {
  const p = PropertiesService.getUserProperties();
  if (data.proxyUrl !== undefined) p.setProperty('LLM_PROXY_URL', String(data.proxyUrl).trim());
  if (data.model !== undefined)    p.setProperty('LLM_MODEL', String(data.model).trim());
  if (data.apiKey !== undefined && String(data.apiKey).trim() !== '' && data.apiKey !== '••••••') {
    p.setProperty('LLM_API_KEY', String(data.apiKey).trim());
  }
  if (data.clearKey === true) p.deleteProperty('LLM_API_KEY');
}
function getApiKey_() { return PropertiesService.getUserProperties().getProperty('LLM_API_KEY') || ''; }

/* ====== ADD-ON UI (CardService) ====== */
function buildAddOnHomePage() {
  const s = getUserSettings_();
  const section = CardService.newCardSection().addWidget(
    CardService.newTextParagraph().setText('<b>LLM Settings</b>')
  );

  section.addWidget(CardService.newTextInput().setFieldName('proxyUrl')
    .setTitle('Proxy URL').setValue(s.proxyUrl).setHint('e.g. https://llm-proxy.mprlab.com/'));

  section.addWidget(CardService.newTextInput().setFieldName('apiKey')
    .setTitle('API Key').setValue(s.apiKeyPresent ? '••••••' : '')
    .setHint(s.apiKeyPresent ? 'Key is saved (leave as •••••• or blank to keep)' : 'Enter your key'));

  section.addWidget(CardService.newSelectionInput().setType(CardService.SelectionInputType.CHECK_BOX)
    .setFieldName('clearKey').setTitle('Security').addItem('Clear saved key on Save', '1', false));

  section.addWidget(CardService.newTextInput().setFieldName('model')
    .setTitle('Default Model').setValue(s.model).setHint('e.g. gpt-5-mini'));

  section.addWidget(CardService.newTextButton().setText('Save')
    .setOnClickAction(CardService.newAction().setFunctionName('handleSave_'))
    .setTextButtonStyle(CardService.TextButtonStyle.FILLED));

  section.addWidget(CardService.newDivider());

  section.addWidget(CardService.newTextButton().setText('Enable functions in this sheet')
    .setOnClickAction(CardService.newAction().setFunctionName('handleInstallBoundFunctions_')));

  section.addWidget(CardService.newTextButton().setText('Grant Network Access')
    .setOnClickAction(CardService.newAction().setFunctionName('handleGrantNetwork_')));

  section.addWidget(CardService.newTextButton().setText('Regenerate Selection')
    .setOnClickAction(CardService.newAction().setFunctionName('handleRegenerate_')));

  section.addWidget(CardService.newTextButton().setText('Retry Errors In Sheet')
    .setOnClickAction(CardService.newAction().setFunctionName('handleRetryErrors_')));

  return CardService.newCardBuilder().addSection(section).build();
}

function handleSave_(e) {
  const inputs = (e && e.commonEventObject && e.commonEventObject.formInputs) || {};
  setUserSettings_({
    proxyUrl: getInput_(inputs, 'proxyUrl') || undefined,
    model:    getInput_(inputs, 'model')    || undefined,
    apiKey:   getInput_(inputs, 'apiKey')   || undefined,
    clearKey: isChecked_(inputs, 'clearKey')
  });
  return CardService.newActionResponseBuilder()
    .setNotification(CardService.newNotification().setText('Saved')).build();
}

function handleGrantNetwork_() {
  const s = getUserSettings_();
  try {
    UrlFetchApp.fetch(s.proxyUrl.replace(/\/?$/, '/'), { method: 'get', muteHttpExceptions: true, followRedirects: true });
    return CardService.newActionResponseBuilder()
      .setNotification(CardService.newNotification().setText('Network access granted.')).build();
  } catch (err) {
    return CardService.newActionResponseBuilder()
      .setNotification(CardService.newNotification().setText('Grant failed: ' + String(err).slice(0, 120))).build();
  }
}

/* ====== ONE-CLICK INSTALL OF BOUND FUNCTIONS ======
   Uses Advanced Services: Script API + Drive API (enabled in manifest). */
function handleInstallBoundFunctions_() {
  try {
    installOrUpdateBoundFunctions_();
    return CardService.newActionResponseBuilder()
      .setNotification(CardService.newNotification().setText('Functions installed. Reload the sheet and use =LLM().')).build();
  } catch (err) {
    return CardService.newActionResponseBuilder()
      .setNotification(CardService.newNotification().setText('Install failed: ' + String(err).slice(0, 180))).build();
  }
}

function installOrUpdateBoundFunctions_() {
  const ssId = SpreadsheetApp.getActive().getId();

  // 1) Find existing bound script, if any.
  const q = `'${ssId}' in parents and mimeType='application/vnd.google-apps.script' and trashed=false`;
  const list = Drive.Files.list({ q: q, fields: 'files(id,name)' }); // v3
  let scriptId = (list.files && list.files.length) ? list.files[0].id : null;

  // 2) Create bound project if none.
  if (!scriptId) {
    const created = Script.Projects.create({ title: 'LLM Sheet Functions', parentId: ssId });
    scriptId = created.scriptId;
  }

  // 3) Prepare our functions file (idempotent upsert).
  const functionsSource = makeLLMFunctionsSource_();
  const existing = Script.Projects.getContent({ scriptId: scriptId });
  const files = existing.files || [];

  // remove any old version of our file by name
  const keep = files.filter(f => f.name !== 'LLMFunctions');
  keep.push({ name: 'LLMFunctions', type: 'SERVER_JS', source: functionsSource });

  // add/merge manifest to ensure scopes + whitelist
  const manifest = {
    timeZone: 'America/Los_Angeles',
    exceptionLogging: 'STACKDRIVER',
    runtimeVersion: 'V8',
    oauthScopes: [
      'https://www.googleapis.com/auth/spreadsheets.currentonly',
      'https://www.googleapis.com/auth/script.external_request'
    ],
    urlFetchWhitelist: [DEFAULT_PROXY_URL]
  };
  // replace or add appsscript.json
  const others = keep.filter(f => f.type !== 'JSON' || f.name !== 'appsscript');
  others.push({ name: 'appsscript', type: 'JSON', source: JSON.stringify(manifest, null, 2) });

  Script.Projects.updateContent({ files: others }, { scriptId: scriptId });
}

/* ====== The code we inject into the bound project ====== */
function makeLLMFunctionsSource_() {
  return `
const DEFAULT_PROXY_URL = '${DEFAULT_PROXY_URL}';
const DEFAULT_MODEL = '${DEFAULT_MODEL}';

function __LLM_getSettings() {
  const p = PropertiesService.getUserProperties();
  return {
    proxyUrl: p.getProperty('LLM_PROXY_URL') || DEFAULT_PROXY_URL,
    apiKey:   p.getProperty('LLM_API_KEY')   || '',
    model:    p.getProperty('LLM_MODEL')     || DEFAULT_MODEL
  };
}

/** =LLM(prompt, [systemPrompt]) */
function LLM(prompt, systemPrompt) { return __LLM_core(prompt, systemPrompt, { model: 'gpt-5-mini' }); }
/** =LLM_WEB(prompt, [systemPrompt]) */
function LLM_WEB(prompt, systemPrompt) { return __LLM_core(prompt, systemPrompt, { model: 'gpt-5-mini', web_search: 1 }); }
/** =LLM_GPT5(prompt, [systemPrompt]) */
function LLM_GPT5(prompt, systemPrompt) { return __LLM_core(prompt, systemPrompt, { model: 'gpt-5' }); }

function __LLM_core(prompt, systemPrompt, overrides) {
  const s = __LLM_getSettings();
  if (!s.apiKey) throw new Error('LLM: API key not set. Open add-on → Save your key.');
  const isRange = Array.isArray(prompt) && Array.isArray(prompt[0]);
  const text = isRange ? prompt.flat().filter(v => v !== '').join('\\n') : (prompt == null ? '' : String(prompt));
  const sys = (systemPrompt == null) ? '' : String(systemPrompt);
  const params = Object.assign({ model: s.model || DEFAULT_MODEL, web_search: 0, format: 'text/plain' }, overrides || {});
  const url = __LLM_buildUrl(s.proxyUrl, {
    key: s.apiKey, prompt: text, system_prompt: sys, model: params.model,
    web_search: params.web_search, format: params.format, nonce: String(Date.now())
  });
  const res = UrlFetchApp.fetch(url, { method: 'get', muteHttpExceptions: true, followRedirects: true });
  if (res.getResponseCode() < 200 || res.getResponseCode() >= 300) {
    throw new Error('LLM HTTP ' + res.getResponseCode() + ': ' + res.getContentText().slice(0, 500));
  }
  const body = res.getContentText();
  return (params.format && params.format.toLowerCase() === 'text/csv') ? Utilities.parseCsv(body) : body;
}
function __LLM_buildUrl(base, p) {
  const q = Object.keys(p).map(k => k + '=' + encodeURIComponent(String(p[k]))).join('&');
  let b = base || DEFAULT_PROXY_URL; if (!b.endsWith('/')) b += '/'; return b + '?' + q;
}
`;
}

/* ====== SHEETS HELPERS USED BY UI BUTTONS ====== */
function handleRegenerate_() { _regen_(); return CardService.newActionResponseBuilder()
  .setNotification(CardService.newNotification().setText('Regenerated selection')).build(); }
function handleRetryErrors_() { _retry_(); return CardService.newActionResponseBuilder()
  .setNotification(CardService.newNotification().setText('Retried error cells')).build(); }
function _regen_() {
  const sheet = SpreadsheetApp.getActiveSheet();
  const ranges = (sheet.getSelection() && sheet.getSelection().getActiveRangeList())
    ? sheet.getSelection().getActiveRangeList().getRanges() : [];
  ranges.forEach((r) => {
    const formulas = r.getFormulas();
    for (let row = 0; row < formulas.length; row++) {
      for (let col = 0; col < (formulas[0] || []).length; col++) {
        const f = formulas[row][col]; if (!f) continue;
        if (/^=(LLM|LLM_WEB|LLM_GPT5)\(/i.test(String(f))) {
          r.getCell(row + 1, col + 1).setFormula(String(f) + ' '); // nudge
        }
      }
    }
  });
}
function _retry_() {
  const sheet = SpreadsheetApp.getActiveSheet();
  const rng = sheet.getDataRange();
  const formulas = rng.getFormulas();
  const displays = rng.getDisplayValues();
  for (let r = 0; r < formulas.length; r++) {
    for (let c = 0; c < (formulas[0] || []).length; c++) {
      if (/^=(LLM|LLM_WEB|LLM_GPT5)\(/i.test(String(formulas[r][c])) && String(displays[r][c]).trim().startsWith('#')) {
        rng.getCell(r + 1, c + 1).setFormula(String(formulas[r][c]) + ' ');
      }
    }
  }
}

/* ====== SMALL UTILS ====== */
function getInput_(inputs, name) {
  if (!inputs || !inputs[name] || !inputs[name].stringInputs) return '';
  const arr = inputs[name].stringInputs.value || []; return arr.length ? arr[0] : '';
}
function isChecked_(inputs, name) {
  if (!inputs || !inputs[name] || !inputs[name].stringInputs) return false;
  const arr = inputs[name].stringInputs.value || []; return arr.indexOf('1') !== -1;
}
