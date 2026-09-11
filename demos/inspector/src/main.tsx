// SPDX-License-Identifier: Apache-2.0

import { createRoot } from 'react-dom/client'
import { App } from './App'
import { Boundary } from './Boundary'

const host = document.getElementById('app')
if (!host) throw new Error('no #app to mount into')
createRoot(host).render(
  <Boundary>
    <App />
  </Boundary>,
)
