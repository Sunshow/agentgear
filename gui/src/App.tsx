import { useEffect } from 'react'
import { MainWindow } from './components/layout/MainWindow'
import { Toaster } from 'sonner'
import { initTheme } from './store/theme-store'

function App() {
  useEffect(() => {
    initTheme()
  }, [])

  return (
    <>
      <MainWindow />
      <Toaster position="bottom-right" />
    </>
  )
}

export default App
