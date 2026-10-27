import { ToolOutlined } from '@ant-design/icons'
import { Card, Empty } from 'antd'
import { PageHeader } from '../components/PageHeader'

// StubPage stands in for a page that is not built yet.
export function StubPage({ title, stage }: { title: string; stage: string }) {
  return (
    <>
      <PageHeader title={title} />
      <Card variant="borderless">
        <Empty
          image={<ToolOutlined style={{ fontSize: 48, color: '#949BA4' }} />}
          description={`Раздел появится на этапе ${stage}`}
        />
      </Card>
    </>
  )
}
