# 08 — Forms & Validation: React 19 Actions

> **Goal:** Choose between controlled and uncontrolled forms, implement client validation, and build modern asynchronous forms with React 19 Action hooks.

---

## 1. Concept: Controlled vs Uncontrolled Forms

React forms handle input data in two ways:

| Feature | Controlled Inputs | Uncontrolled Inputs |
|---------|-------------------|---------------------|
| **Source of Truth** | React State (`useState`). | The Browser DOM. |
| **Updates** | Updates state on every keystroke (`onChange`). | Read on demand (via `useRef` or `FormData`). |
| **Re-renders** | Re-renders component on *every keypress*. | No re-renders when typing. |
| **Ideal For** | Live validation, instant inputs formatting. | Simple submissions, large forms (performance). |

---

## 2. Mechanism: React 19 Form Actions

React 19 upgrades form management by introducing **Actions**. Instead of writing manual async handles:

```typescript
// Legacy form handle
async function handleSubmit(e) {
  e.preventDefault();
  setIsPending(true);
  await saveForm(data);
  setIsPending(false);
}
```

You can pass an async function directly to the HTML `<form action={...}>` prop. React automatically manages the transition pending state.

### React 19 Action Hooks
- **`useActionState`:** Manages action returns, error states, and pending indicators.
- **`useFormStatus`:** Subscribes to the parent form's submission status (e.g. pending) inside child components.

---

## 3. Variations & Depth: Schema Validation

In production, you should separate validation logic from form rendering code. Schema validators (like Zod or Yup) let you declare form requirements in a clean schema, validating incoming `FormData` dynamically.

```typescript
import { z } from 'zod';
const userSchema = z.object({
  username: z.string().min(3, 'Username must be at least 3 characters'),
  email: z.string().email('Invalid email address')
});
```

---

## 4. Practical Application: Modern Asynchronous Form

Let's write a registration form using React 19's `useActionState` and a nested submit button utilizing `useFormStatus`.

**`RegisterForm.tsx`**
```tsx
import React, { useActionState } from 'react';
import { useFormStatus } from 'react-dom';

// Simulated async server action
async function registerUser(prevState: { error?: string; success?: boolean } | null, formData: FormData) {
  const email = formData.get('email') as string;
  const password = formData.get('password') as string;

  // Simulate network latency
  await new Promise((resolve) => setTimeout(resolve, 1500));

  if (!email.includes('@')) {
    return { error: 'Invalid email address.' };
  }
  if (password.length < 6) {
    return { error: 'Password must be at least 6 characters.' };
  }

  return { success: true };
}

// Child Submit Button consuming form status
function SubmitButton() {
  const { pending } = useFormStatus();

  return (
    <button type="submit" disabled={pending}>
      {pending ? 'Submitting registration...' : 'Register'}
    </button>
  );
}

export default function RegisterForm() {
  // useActionState hooks into the action function and tracks state
  const [state, formAction] = useActionState(registerUser, null);

  return (
    <div style={{ maxWidth: '300px', padding: '20px', border: '1px solid #ccc', borderRadius: '6px' }}>
      <h3>Create Account</h3>
      <form action={formAction} style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
        <input 
          name="email" 
          type="text" 
          placeholder="Email address" 
          required 
          style={{ padding: '6px' }}
        />
        <input 
          name="password" 
          type="password" 
          placeholder="Password" 
          required 
          style={{ padding: '6px' }}
        />

        {state?.error && <p style={{ color: 'red', fontSize: '14px', margin: 0 }}>{state.error}</p>}
        {state?.success && <p style={{ color: 'green', fontSize: '14px', margin: 0 }}>Success!</p>}

        <SubmitButton />
      </form>
    </div>
  );
}
```

---

## 5. Common Mistakes & Gotchas

- **Controlled inputs lag in large forms:** If a parent component holds 30 controlled states and updates them on every keystroke, typing will lag. Wrap inputs in separate components, or use uncontrolled `FormData` instead.
- **Forgetting `name` attributes on input tags:** When using uncontrolled forms or Actions, inputs *must* have a `name` attribute. Otherwise, `new FormData(formElement)` will ignore them.
- **Using `useFormStatus` in the same component declaring the form:** `useFormStatus` only tracks forms *above* it in the component hierarchy. It must be called inside a child component nested within the `<form>` tag.

---

## 🎯 Key Takeaways

- **React 19 Actions automate pending states** and clean up async error trackers.
- **Use `useFormStatus` in child elements** to disable buttons or trigger submit overlays.
- **Provide `name` tags** on all inputs to take advantage of native HTML form collection APIs.

---

*← [data fetching](./07_data_fetching_patterns.md) | [next → 09 Testing & Component Design Patterns](./09_testing_and_production_patterns.md)*
