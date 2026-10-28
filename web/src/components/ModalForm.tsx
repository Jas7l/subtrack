import { Form, type FormProps } from 'antd'
import { type ReactNode, useLayoutEffect } from 'react'

// ResetOnMount puts the form back to its initialValues once its fields have
// mounted. It must come after them: a field registers on mount, and a reset
// before that would not redraw it.
function ResetOnMount() {
  const form = Form.useFormInstance()
  useLayoutEffect(() => {
    form.resetFields()
  }, [form])
  return null
}

// ModalForm is the Form of a modal: it starts from initialValues every time
// the modal opens. The form instance lives in the modal's component, which
// stays mounted while the modal is hidden, so a plain Form would come back
// with what was typed into it last time (or another record's values) and save
// them. clearOnDestroy fixes that in a build but empties the form on open
// under StrictMode, which unmounts and remounts it in development.
export function ModalForm<Values>({ children, ...props }: FormProps<Values>) {
  return (
    <Form<Values> {...props}>
      {children as ReactNode}
      <ResetOnMount />
    </Form>
  )
}
