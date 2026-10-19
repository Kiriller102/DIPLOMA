# Интерфейс DIPLOMA

Запуск всего проекта и ограничения деморежима описаны в [корневом README](../../README.md).

Для разработки запустите Go API на `localhost:8080`, затем выполните в этой папке:

```sh
npm ci
npm run dev
```

Vite проксирует `/api` к Go; в Docker это делает Nginx. Проверки: `npm run lint` и `npm run build`. Компоненты Auth0 сохранены как прототип, рабочий интерфейс использует выбор демопользователя.

## Исходный шаблон React + Vite

This template provides a minimal setup to get React working in Vite with HMR and some ESLint rules.

Currently, two official plugins are available:

- [@vitejs/plugin-react](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react/README.md) uses [Babel](https://babeljs.io/) for Fast Refresh
- [@vitejs/plugin-react-swc](https://github.com/vitejs/vite-plugin-react-swc) uses [SWC](https://swc.rs/) for Fast Refresh

## Expanding the ESLint configuration

If you are developing a production application, we recommend using TypeScript and enable type-aware lint rules. Check out the [TS template](https://github.com/vitejs/vite/tree/main/packages/create-vite/template-react-ts) to integrate TypeScript and [`typescript-eslint`](https://typescript-eslint.io) in your project.
