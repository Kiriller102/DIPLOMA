const assert = require('node:assert/strict');
const http = require('node:http');
const { chromium } = require('playwright');
let base = process.env.WEB_URL || 'http://localhost:5173';
const password = 'e2e-safe-password-123';
const secretPhrase = index => `Отдельная секретная фраза устройства ${index} 🔐`;
async function unlock(page, index, setup = false) {
  await page.getByLabel('Секретная фраза', { exact: true }).fill(secretPhrase(index));
  if (setup) await page.getByLabel('Повторите секретную фразу', { exact: true }).fill(secretPhrase(index));
  await page.getByRole('button', { name: setup ? 'Настроить шифрование' : 'Открыть переписку', exact: true }).click();
  try { await page.getByRole('button', { name: 'Создать чат', exact: true }).waitFor(); }
  catch (error) { error.message += `\nEncryption screen: ${await page.locator('body').innerText()}`; throw error; }
}
async function api(path, method = 'GET', data, token) {
  const response = await fetch(`${base}/api${path}`, {
    method, headers: { 'Content-Type': 'application/json', 'X-Messenger-Request': '1', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    body: data === undefined ? undefined : JSON.stringify(data),
  });
  const result = response.status === 204 ? null : await response.json();
  assert.ok(response.ok, `${method} ${path}: ${JSON.stringify(result)}`);
  return result;
}
(async () => {
  const users = [];
  let chatID;
  let browser;
  let proxy;
  const errors = [];
  const outgoing = [];
  try {
    // Give Docker CI a genuine secure context (loopback), without disabling
    // browser security. Public deployments must use HTTPS.
    const target = new URL(base);
    if (target.protocol === 'http:' && !['localhost', '127.0.0.1', '[::1]'].includes(target.hostname)) {
      proxy = http.createServer((request, response) => {
        const upstream = http.request({ hostname: target.hostname, port: target.port || 80,
          path: request.url, method: request.method, headers: request.headers }, result => {
          response.writeHead(result.statusCode, result.headers); result.pipe(response);
        });
        upstream.on('error', () => { response.writeHead(502); response.end(); });
        response.on('close', () => upstream.destroy());
        request.pipe(upstream);
      });
      await new Promise(resolve => proxy.listen(0, '127.0.0.1', resolve));
      base = `http://127.0.0.1:${proxy.address().port}`;
    }
    browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] });
    const first = await browser.newPage();
    const second = await browser.newPage();
    const suffix = Date.now();
    for (const [index, page] of [first, second].entries()) {
      page.on('pageerror', error => errors.push(error.message));
      page.on('request', request => { if (request.method() === 'POST') outgoing.push(request.postData() || ''); });
      const username = `e2e_${index}_${suffix}`;
      const email = `${username}@example.com`;
      await page.goto(base);
      await page.getByRole('button', { name: 'Создать аккаунт', exact: true }).click();
      await page.getByLabel('Имя пользователя', { exact: true }).fill(username);
      await page.getByLabel('Email', { exact: true }).fill(email);
      await page.getByLabel('Пароль', { exact: true }).fill(password);
      await page.getByRole('button', { name: 'Зарегистрироваться', exact: true }).click();
      await page.getByLabel('Секретная фраза', { exact: true }).waitFor();
      const { access_token: token, user } = await api('/auth/token', 'POST', { email, password });
      users.push({ ...user, token, email });
      await unlock(page, index, true);
    }
    // Refresh the directory after the second registration.
    await first.reload();
    await unlock(first, 0);
    const name = `Браузерный чат ${suffix}`;
    await first.getByLabel('Название чата').fill(name);
    await first.getByLabel('Участник нового чата').selectOption(String(users[1].id));
    await first.getByRole('button', { name: 'Создать чат', exact: true }).click();
    await first.getByRole('button', { name, exact: true }).waitFor();
    const chats = await api(`/chat_participant/chats/${users[0].id}`, 'GET', undefined, users[0].token);
    chatID = chats.find(chat => chat.name === name).id;
    await second.getByRole('button', { name, exact: true }).click({ timeout: 15000 });
    const text = `Привет 👋 100% + <script> & ${suffix}`;
    await first.getByRole('textbox', { name: 'Сообщение', exact: true }).fill(text);
    await first.getByRole('button', { name: 'Отправить сообщение' }).click();
    await second.getByText(text, { exact: true }).waitFor({ timeout: 10000 });
    assert.equal(await second.getByText(text, { exact: true }).count(), 1);
    const reply = `Ответ ${suffix}`;
    await second.getByLabel('Алгоритм шифрования').selectOption('Kuznechik');
    await second.getByRole('textbox', { name: 'Сообщение', exact: true }).fill(reply);
    await second.getByRole('button', { name: 'Отправить сообщение' }).click();
    await first.getByText(reply, { exact: true }).waitFor({ timeout: 10000 });
    const stored = await api(`/messages/${chatID}`, 'GET', undefined, users[0].token);
    assert.equal(stored.messages.length, 2);
    assert.deepEqual(stored.messages.map(message => message.encryption_algorithm), ['AES', 'Kuznechik']);
    for (const message of stored.messages) {
      assert.equal(message.crypto_version, 1);
      assert.ok(message.content.startsWith('e2ee:v1:'));
      assert.ok(!message.content.includes(text) && !message.content.includes(reply));
    }
    assert.ok(!outgoing.some(body => body.includes(text) || body.includes(reply) || body.includes(secretPhrase(0)) || body.includes(secretPhrase(1))));
    await first.reload();
    await unlock(first, 0);
    await first.getByRole('button', { name, exact: true }).click();
    await first.getByText(text, { exact: true }).waitFor();
    await first.getByText(reply, { exact: true }).waitFor();
    await first.setViewportSize({ width: 390, height: 844 });
    await first.locator('label.toggle').click();
    assert.ok(await first.getByRole('textbox', { name: 'Сообщение', exact: true }).isVisible());
    await first.getByRole('button', { name: 'Выйти', exact: true }).click();
    await first.getByRole('button', { name: 'Войти', exact: true }).waitFor();
    assert.equal(await first.getByText(text, { exact: true }).count(), 0);
    await first.getByLabel('Email', { exact: true }).fill(users[0].email);
    await first.getByLabel('Пароль', { exact: true }).fill(password);
    await first.getByRole('button', { name: 'Войти', exact: true }).click();
    await unlock(first, 0);
    await first.getByRole('button', { name, exact: true }).click();
    await first.getByText(reply, { exact: true }).waitFor();
    // A fresh browser/device has neither local keys nor trusted-key pins.
    const third = await browser.newPage();
    third.on('pageerror', error => errors.push(error.message));
    await third.goto(base);
    await third.getByLabel('Email', { exact: true }).fill(users[0].email);
    await third.getByLabel('Пароль', { exact: true }).fill(password);
    await third.getByRole('button', { name: 'Войти', exact: true }).click();
    await third.getByLabel('Секретная фраза', { exact: true }).fill('Неверная секретная фраза');
    await third.getByRole('button', { name: 'Открыть переписку', exact: true }).click();
    await third.getByRole('alert').filter({ hasText: 'Неверная секретная фраза' }).waitFor();
    await unlock(third, 0);
    await third.getByRole('button', { name, exact: true }).click();
    await third.getByText(text, { exact: true }).waitFor();
    await third.getByText(reply, { exact: true }).waitFor();
    assert.deepEqual(errors, []);
    console.log('Browser E2E: AES + Kuznechik, ciphertext-only API, protected keys, second device, Unicode, mobile, reload and sessions OK');
  } finally {
    if (browser) await browser.close();
    try {
      if (chatID && users[0]) await api(`/chat/${chatID}`, 'DELETE', undefined, users[0].token);
      for (const user of users) await api(`/users/${user.id}`, 'DELETE', undefined, user.token);
    } finally {
      if (proxy) { proxy.closeAllConnections(); await new Promise(resolve => proxy.close(resolve)); }
    }
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
