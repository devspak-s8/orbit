import { useEffect, useState, type FormEvent } from 'react'
import { useParams } from 'react-router-dom'
import { ApiError, submitPublicForm } from '../lib/api'

/**
 * PublicForm is the public, unauthenticated face of a Data table: it
 * renders a form and submits to POST /forms/{token}. The secret is the
 * URL; the experience is "fill and done".
 */
export default function PublicForm() {
  const { token } = useParams<{ token: string }>()
  const [values, setValues] = useState<Record<string, unknown>>({})
  const [state, setState] = useState<'ready' | 'sent' | 'error'>('ready')
  const [message, setMessage] = useState('')

  useEffect(() => {
    // Nothing to fetch: the form shape ships with the page and the
    // submit endpoint validates against the table's real columns.
  }, [token])

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!token) return
    try {
      await submitPublicForm(token, values)
      setState('sent')
    } catch (err) {
      if (err instanceof ApiError && err.code === 'invalid_values') {
        setMessage(err.message)
      } else {
        setMessage(err instanceof ApiError ? err.message : 'Something went wrong')
      }
      setState('error')
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-neutral-950 px-4 py-10 text-neutral-100">
      <div className="w-full max-w-md">
        <div className="rounded-xl border border-neutral-800 bg-neutral-900/60 p-8">
          <div className="mb-6 text-lg font-semibold tracking-widest">ORBIT</div>
          {state === 'sent' && (
            <div className="space-y-3 text-center">
              <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full border border-emerald-700 bg-emerald-900/30 text-emerald-300">
                ✓
              </div>
              <h1 className="text-lg font-medium">Thank you!</h1>
              <p className="text-sm text-neutral-400">Your submission was received.</p>
            </div>
          )}
          {state === 'error' && (
            <div className="space-y-3">
              <h1 className="text-lg font-medium">We couldn't take that.</h1>
              <p className="text-sm text-red-400">{message}</p>
              <button
                onClick={() => setState('ready')}
                className="rounded border border-neutral-700 px-3 py-1.5 text-sm hover:border-neutral-500"
              >
                Try again
              </button>
            </div>
          )}
          {state === 'ready' && (
            <form onSubmit={handleSubmit} className="space-y-4">
              <div>
                <h1 className="text-lg font-medium">Send us a message</h1>
                <p className="mt-1 text-sm text-neutral-500">
                  Fill in the fields and submit — it goes straight into the team's workspace.
                </p>
              </div>
              {/* The alpha form ships a name/email/message trio; the
                  submit endpoint validates against the real columns. */}
              <FieldsFromValues values={values} setValues={setValues} />
              <button
                type="submit"
                className="w-full rounded-lg bg-emerald-500 px-4 py-2.5 text-sm font-semibold text-black hover:bg-emerald-400"
              >
                Submit
              </button>
            </form>
          )}
        </div>
        <p className="mt-4 text-center text-xs text-neutral-600">
          Powered by <span className="text-neutral-400">ORBIT</span> — the open operations platform
        </p>
      </div>
    </div>
  )
}

/**
 * The alpha public form renders a name/email/message trio by default.
 * Column-aware rendering arrives with the form builder; the submit
 * endpoint validates against the real columns either way.
 */
function FieldsFromValues({
  values,
  setValues,
}: {
  values: Record<string, unknown>
  setValues: (v: Record<string, unknown>) => void
}) {
  const fields: Array<[string, string, string]> = [
    ['name', 'Your name', 'text'],
    ['email', 'Email', 'email'],
    ['message', 'Message', 'text'],
  ]
  return (
    <>
      {fields.map(([key, label, type]) => (
        <label key={key} className="block text-sm">
          {label}
          <input
            type={type}
            required={key !== 'message'}
            value={String(values[key] ?? '')}
            onChange={(e) => setValues({ ...values, [key]: e.target.value })}
            className="mt-1 w-full rounded border border-neutral-700 bg-neutral-950 px-3 py-2 text-sm focus:border-emerald-600 focus:outline-none"
          />
        </label>
      ))}
    </>
  )
}