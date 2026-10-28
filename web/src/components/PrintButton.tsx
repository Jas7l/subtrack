import { FilePdfOutlined } from '@ant-design/icons'
import { Button } from 'antd'

// PrintButton: «PDF» through the browser's print dialog («Сохранить как
// PDF»). The print styles in index.css drop the menu and buttons and keep
// the dark page as on screen.
export function PrintButton() {
  return (
    <Button icon={<FilePdfOutlined />} onClick={() => window.print()} className="no-print">
      PDF
    </Button>
  )
}
