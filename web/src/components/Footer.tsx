import { useEffect, useState } from 'preact/hooks'
import { api } from '../api/client'

// Module-level cache: every view mounts its own <Footer>, so this avoids
// re-fetching /api/version on every navigation.
let cachedVersion: string | null = null

export function Footer() {
  const [version, setVersion] = useState(cachedVersion)

  useEffect(() => {
    if (cachedVersion !== null) return
    api
      .version()
      .then((res) => {
        cachedVersion = res.version
        setVersion(res.version)
      })
      .catch(() => {})
  }, [])

  if (!version) return null

  return <footer class="app-footer">aiinventory {version}</footer>
}
