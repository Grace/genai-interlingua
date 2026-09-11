// SPDX-License-Identifier: Apache-2.0

import { Component, type ErrorInfo, type ReactNode } from 'react'

/**
 * Catches a render error and says so on the page.
 *
 * React's default for an uncaught error is to unmount the whole tree, which
 * renders an empty document. That is the worst failure a page can have: the
 * reader gets a blank screen with no indication anything went wrong, and the
 * only trace of it is in a console they have no reason to open. It happened
 * here -- a nil slice arrived as null, .length threw, and one capture rendered
 * nothing at all.
 *
 * Fixing that particular bug does not remove the need for this. A page with a
 * dozen states and a WASM binary behind it will have another one, and the
 * difference between "this broke, here is what it said" and a white rectangle
 * is the difference between a reader filing something useful and a reader
 * closing the tab.
 */
export class Boundary extends Component<{ children: ReactNode }, { error?: Error }> {
  state: { error?: Error } = {}

  static getDerivedStateFromError(error: Error) {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // Kept for anyone who does open the console: the component stack names the
    // component that threw, which the message alone does not.
    console.error('inspector: render failed', error, info.componentStack)
  }

  render() {
    const { error } = this.state
    if (!error) return this.props.children

    return (
      <div className="notice error">
        <h2>The inspector hit an error while rendering.</h2>
        <p>
          <code>{error.message}</code>
        </p>
        <p>
          This is a bug in the page, not in the normalizer — the translation
          itself may well have succeeded. Reload to try another capture.
        </p>
        {error.stack && (
          <details>
            <summary>Stack</summary>
            <pre>{error.stack}</pre>
          </details>
        )}
      </div>
    )
  }
}
