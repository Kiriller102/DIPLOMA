const assert = require('node:assert/strict');
const { chromium } = require('playwright');

// Requires the running Compose stack; creates and removes its own demo records.
const base = process.env.WEB_URL || 'http://localhost:5173';
async function api(path, method = 'GET', data) {
  const response = await fetch(`${base}/api${path}`, {
    method, headers: { 'Content-Type': 'application/json' },
    body: data === undefined ? undefined : JSON.stringify(data),
  });
  const result = await response.json();
  assert.ok(response.ok, `${method} ${path}: ${JSON.stringify(result)}`);
  return result;
}

(async () => {
  const users = [];
  let chatID;
  let browser;
  const errors = [];
  try {
    const suffix = Date.now();
    for (const name of ['first', 'second']) {
      const { user } = await api('/users', 'POST', {
        username: `e2e_${name}_${suffix}`, email: `${name}_${suffix}@example.com`,
        password_hash: '!e2e-demo-only', is_active: true,
      });
      users.push(user);
    }
    browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] });
    const first = await browser.newPage();
    const second = await browser.newPage();
    for (const page of [first, second]) {
      page.on('pageerror', error => errors.push(error.message));
      await page.goto(base);
    }
    await first.getByLabel('Пользователь', { exact: true }).selectOption(String(users[0].id));
    await second.getByLabel('Пользователь', { exact: true }).selectOption(String(users[1].id));
    const name = `Браузерный чат ${suffix}`;
    await first.getByLabel('Название чата').fill(name);
    await first.getByLabel('Участник нового чата').selectOption(String(users[1].id));
    await first.getByRole('button', { name: 'Создать чат', exact: true }).click();
    await first.getByRole('button', { name, exact: true }).waitFor();
    const chats = await api(`/chat_participant/chats/${users[0].id}`);
    chatID = chats.find(chat => chat.name === name).id;
    await second.getByRole('button', { name, exact: true }).click({ timeout: 15000 });
    const text = `Привет 👋 100% + <script> & ${suffix}`;
    await first.getByRole('textbox', { name: 'Сообщение', exact: true }).fill(text);
    await first.getByRole('button', { name: 'Отправить сообщение' }).click();
    await second.getByText(text, { exact: true }).waitFor({ timeout: 10000 });
    assert.equal(await second.getByText(text, { exact: true }).count(), 1);
    const reply = `Ответ ${suffix}`;
    await second.getByRole('textbox', { name: 'Сообщение', exact: true }).fill(reply);
    await second.getByRole('button', { name: 'Отправить сообщение' }).click();
    await first.getByText(reply, { exact: true }).waitFor({ timeout: 10000 });
    await first.reload();
    await first.getByRole('button', { name, exact: true }).click();
    await first.getByText(text, { exact: true }).waitFor();
    await first.getByText(reply, { exact: true }).waitFor();
    // A narrow viewport must keep navigation and the composer usable.
    await first.setViewportSize({ width: 390, height: 844 });
    await first.locator('label.toggle').click();
    await first.getByRole('textbox', { name: 'Сообщение', exact: true }).waitFor();
    assert.deepEqual(errors, []);
    console.log('Browser E2E: chat creation, two users, Unicode, replies, reload and mobile layout OK');
  } finally {
    if (browser) await browser.close();
    if (chatID) await api(`/chat/${chatID}`, 'DELETE');
    for (const user of users) await api(`/users/${user.id}`, 'DELETE');
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
