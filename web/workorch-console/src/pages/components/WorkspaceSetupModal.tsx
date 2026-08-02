import { useEffect, useState } from 'react';
import { App as AntApp, Button, Descriptions, Empty, Form, Input, List, Modal, Select, Space, Steps, Tag, Typography } from 'antd';
import { ProCard } from '../../components/proCompat';
import { workorchApi } from '../../api';
import type { Soul, WorkspaceSetupDraftSession, WorkspaceTemplate } from '../../types';
import {
  boundedParagraph,
  compactTags,
  errorMessage,
  hasWorkspaceSetupDraftContent,
  hasWorkspaceSetupDraftInfoItems,
  joinLines,
  normalizeSoulBindingFormValues,
  setupDoDItems,
  soulBindingStageLabel,
  stageBindings,
  workspaceSetupDraftConfidenceLabel,
  workspaceSetupDraftDoDList,
  workspaceSetupDraftFromForms,
  workspaceSetupDraftInfoList,
  workspaceSetupDraftMessageLabel,
  workspaceSetupDraftReadyForApply,
  workspaceSetupDraftStateColor,
  workspaceSetupDraftStateLabel,
  workspaceSetupDraftTextBlock,
  workspaceSetupSmartDraftStep,
  defaultSoulBindingStages
} from '../../helpers';
import type { DoDSetupFormValues, GoalSetupFormValues, SoulBindingFormValues, WorkspaceSetupFormValues, WorkspaceSetupMode } from '../../helpers';

type WorkspaceSetupModalProps = {
  open: boolean;
  onClose: () => void;
  onCreated: (workspaceId: string) => void | Promise<void>;
};

export function WorkspaceSetupModal({ open, onClose, onCreated }: WorkspaceSetupModalProps) {
  const { message } = AntApp.useApp();
  const [souls, setSouls] = useState<Soul[]>([]);
  const [workspaceTemplates, setWorkspaceTemplates] = useState<WorkspaceTemplate[]>([]);
  const [loadingOptions, setLoadingOptions] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [setupStep, setSetupStep] = useState(0);
  const [setupMode, setSetupMode] = useState<WorkspaceSetupMode>('manual');
  const [draftSession, setDraftSession] = useState<WorkspaceSetupDraftSession>();
  const [draftInput, setDraftInput] = useState('');
  const [draftLoading, setDraftLoading] = useState(false);
  const [draftApplied, setDraftApplied] = useState(false);
  const [workspaceForm] = Form.useForm<WorkspaceSetupFormValues>();
  const [goalForm] = Form.useForm<GoalSetupFormValues>();
  const [doDForm] = Form.useForm<DoDSetupFormValues>();
  const [soulBindingsForm] = Form.useForm<SoulBindingFormValues>();
  const workspaceTitle = Form.useWatch('title', workspaceForm);

  const setupFlow = setupMode === 'smart' ? [0, workspaceSetupSmartDraftStep, 3, 4] : [0, 1, 2, 3, 4];
  const currentIndex = Math.max(0, setupFlow.indexOf(setupStep));
  const canGoPrevious = currentIndex > 0;
  const canGoNext = currentIndex < setupFlow.length - 1;
  const canUseDraft = Boolean(String(workspaceTitle ?? '').trim());
  const soulOptions = souls.map((item) => ({ value: item.id, label: `${item.name || item.id} (${item.id})` }));
  const soulNameById = new Map(souls.map((item) => [item.id, item.name ? `${item.name} (${item.id})` : item.id]));
  const stepLabels: Record<number, string> = {
    0: '工作空间',
    1: '目标',
    2: '验收标准',
    3: '执行人格',
    4: '确认提交',
    [workspaceSetupSmartDraftStep]: '智能向导'
  };

  const clearDraft = () => {
    setDraftSession(undefined);
    setDraftInput('');
    setDraftApplied(false);
  };

  const reset = () => {
    setSetupStep(0);
    setSetupMode('manual');
    workspaceForm.resetFields();
    goalForm.resetFields();
    doDForm.resetFields();
    soulBindingsForm.resetFields();
    clearDraft();
  };

  const close = () => {
    reset();
    onClose();
  };

  useEffect(() => {
    if (!open) {
      return;
    }
    let disposed = false;
    setLoadingOptions(true);
    Promise.all([workorchApi.listSouls(), workorchApi.listWorkspaceTemplates()])
      .then(([nextSouls, nextTemplates]) => {
        if (!disposed) {
          setSouls(nextSouls);
          setWorkspaceTemplates(nextTemplates);
          reset();
        }
      })
      .catch((error: unknown) => {
        if (!disposed) {
          message.error(errorMessage(error));
        }
      })
      .finally(() => {
        if (!disposed) {
          setLoadingOptions(false);
        }
      });
    return () => { disposed = true; };
  }, [open]);

  const onFormValuesChange = (changedValues: Partial<WorkspaceSetupFormValues>) => {
    if (changedValues.setupMode) {
      const nextMode = changedValues.setupMode;
      setSetupMode(nextMode);
      const nextFlow = nextMode === 'smart' ? [0, workspaceSetupSmartDraftStep, 3, 4] : [0, 1, 2, 3, 4];
      if (!nextFlow.includes(setupStep)) {
        setSetupStep(0);
      }
      return;
    }
    if ('title' in changedValues || 'description' in changedValues || 'deliverableDirectory' in changedValues) {
      setDraftSession(undefined);
      setDraftApplied(false);
    }
  };

  const applyTemplate = (templateID?: string) => {
    clearDraft();
    const template = workspaceTemplates.find((item) => item.id === templateID);
    if (!template) {
      goalForm.resetFields();
      doDForm.resetFields();
      soulBindingsForm.resetFields();
      return;
    }
    goalForm.setFieldsValue({ text: template.goalText || '' });
    doDForm.setFieldsValue({ items: joinLines(template.doDItems) });
    soulBindingsForm.setFieldsValue({ defaultSoulId: template.defaultSoulId, bindings: template.soulBindings ?? {} });
  };

  const draftFromForms = () => workspaceSetupDraftFromForms(goalForm.getFieldValue('text'), doDForm.getFieldValue('items'));

  const ensureDraftSession = async () => {
    const values = workspaceForm.getFieldsValue();
    if (
      draftSession && draftSession.templateId === values.templateId && draftSession.workspace?.title === values.title &&
      (draftSession.workspace?.description || '') === (values.description || '') &&
      (draftSession.workspace?.deliverableDirectory || '') === (values.deliverableDirectory || '')
    ) {
      return draftSession;
    }
    if (!values.title?.trim()) {
      throw new Error('请先填写工作空间标题');
    }
    const session = await workorchApi.createWorkspaceSetupDraftSession({
      templateId: values.templateId,
      title: values.title,
      description: values.description,
      deliverableDirectory: values.deliverableDirectory,
      dialogLanguage: typeof navigator !== 'undefined' ? navigator.language : undefined
    });
    setDraftSession(session);
    return session;
  };

  const generateOrReviseDraft = async () => {
    const content = draftInput.trim();
    if (!content && !draftSession) {
      message.warning('请先输入需求描述');
      return;
    }
    if (!content && !hasWorkspaceSetupDraftContent(draftSession?.draft)) {
      message.warning('请先输入需求描述或补充信息');
      return;
    }
    setDraftLoading(true);
    try {
      const session = await ensureDraftSession();
      const values = workspaceForm.getFieldsValue();
      const draft = draftApplied ? draftFromForms() : draftSession?.draft ?? session.draft;
      const request = {
        templateId: values.templateId,
        title: values.title,
        description: values.description,
        deliverableDirectory: values.deliverableDirectory,
        draft,
        dialogLanguage: typeof navigator !== 'undefined' ? navigator.language : undefined
      };
      const updated = content
        ? await workorchApi.appendWorkspaceSetupDraftMessage(session.id, { ...request, content })
        : await workorchApi.regenerateWorkspaceSetupDraft(session.id, request);
      setDraftSession(updated);
      setDraftApplied(false);
      setDraftInput('');
      message.success(content ? 'Goal / DoD 草案已根据补充信息更新' : 'Goal / DoD 草案已重新生成');
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setDraftLoading(false);
    }
  };

  const applyDraft = () => {
    const draft = draftSession?.draft;
    if (!draft) {
      message.warning('暂无可应用的草案');
      return;
    }
    if (draft.goalText) {
      goalForm.setFieldsValue({ text: draft.goalText });
    }
    if (draft.doDItems?.length) {
      doDForm.setFieldsValue({ items: joinLines(draft.doDItems) });
    }
    setDraftApplied(true);
    message.success('已应用草案到目标和验收标准表单');
  };

  const validateStep = async (step: number) => {
    if (step === 0) {
      await workspaceForm.validateFields();
    } else if (step === workspaceSetupSmartDraftStep && setupMode === 'smart') {
      const goalText = String(goalForm.getFieldValue('text') ?? '').trim();
      if (!draftApplied || !goalText || setupDoDItems(doDForm.getFieldValue('items')).length === 0) {
        throw new Error('请先生成并应用 Goal 和 DoD 草案，或返回基础信息切换为手动填写');
      }
    } else if (step === 1) {
      await goalForm.validateFields();
    } else if (step === 2) {
      const values = await doDForm.validateFields();
      if (!setupDoDItems(values.items).length) {
        throw new Error('请至少填写一个验收项');
      }
    } else if (step === 3) {
      await soulBindingsForm.validateFields();
    }
  };

  const next = async () => {
    try {
      await validateStep(setupStep);
      setSetupStep(setupFlow[Math.min(currentIndex + 1, setupFlow.length - 1)] ?? 0);
    } catch (error) {
      message.error(errorMessage(error));
    }
  };

  const previous = () => setSetupStep(setupFlow[Math.max(currentIndex - 1, 0)] ?? 0);

  const submit = async () => {
    setSubmitting(true);
    try {
      await validateStep(0);
      await validateStep(1);
      await validateStep(2);
      await validateStep(3);
      const workspaceValues = workspaceForm.getFieldsValue();
      const workspace = await workorchApi.createWorkspace({
        title: workspaceValues.title,
        description: workspaceValues.description,
        deliverableDirectory: workspaceValues.deliverableDirectory,
        templateId: workspaceValues.templateId
      });
      await workorchApi.defineGoal(workspace.id, goalForm.getFieldValue('text'));
      await workorchApi.defineDoD(workspace.id, setupDoDItems(doDForm.getFieldValue('items')));
      await workorchApi.updateWorkspaceSoulBindings(workspace.id, normalizeSoulBindingFormValues(soulBindingsForm.getFieldsValue()));
      close();
      message.success('工作空间、目标、验收标准和执行人格配置已提交');
      await onCreated(workspace.id);
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      title="创建工作空间"
      open={open}
      confirmLoading={submitting}
      onCancel={close}
      width={1080}
      footer={[
        <Button key="cancel" onClick={close}>取消</Button>,
        canGoPrevious ? <Button key="previous" onClick={previous}>上一步</Button> : null,
        canGoNext ? <Button key="next" type="primary" loading={loadingOptions} onClick={() => void next()}>下一步</Button> : null,
        setupStep === 4 ? <Button key="submit" type="primary" loading={submitting} onClick={() => void submit()}>确认提交</Button> : null
      ].filter(Boolean)}
    >
      <Steps current={currentIndex} className="workorch-section" items={setupFlow.map((step) => ({ title: stepLabels[step] }))} />
      <Form form={workspaceForm} layout="vertical" initialValues={{ setupMode: 'manual' }} onValuesChange={onFormValuesChange} style={{ display: setupStep === 0 ? 'block' : 'none' }}>
        <Form.Item name="setupMode" label="配置方式">
          <Select options={[{ value: 'manual', label: '手动填写 Goal / DoD' }, { value: 'smart', label: '智能生成 Goal / DoD' }]} />
        </Form.Item>
        <Form.Item name="templateId" label="工作空间模板（Workspace Template）">
          <Select allowClear placeholder="可选。选择后自动带入目标、验收标准和执行人格绑定" onChange={applyTemplate} options={workspaceTemplates.map((item) => ({ value: item.id, label: `${item.name || item.id} (${item.id})` }))} />
        </Form.Item>
        <Form.Item name="title" label="标题" rules={[{ required: true }]}><Input placeholder="本地 workorch 工作空间" /></Form.Item>
        <Form.Item name="description" label="描述"><Input.TextArea rows={3} placeholder="描述工作空间目标、范围或当前交付预期" /></Form.Item>
        <Form.Item name="deliverableDirectory" label="工作目录"><Input placeholder="留空时自动使用 .workorch-workspaces/工作空间 ID" /></Form.Item>
      </Form>
      <div style={{ display: setupStep === workspaceSetupSmartDraftStep ? 'block' : 'none' }}>
        <ProCard title="智能草拟 Goal / DoD" className="workorch-section workspace-draft-card" extra={<Space size="small"><Tag color={workspaceSetupDraftStateColor(draftSession?.state)}>{workspaceSetupDraftStateLabel(draftSession?.state)}</Tag>{draftApplied ? <Tag color="green">已应用</Tag> : null}</Space>}>
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <div className="workspace-draft-summary">
              <div><Typography.Text type="secondary">工作空间</Typography.Text><Typography.Text strong>{workspaceForm.getFieldValue('title') || '待创建，最终确认提交后生成'}</Typography.Text></div>
              <div><Typography.Text type="secondary">模板</Typography.Text><Typography.Text>{workspaceForm.getFieldValue('templateId') || '未选择'}</Typography.Text></div>
              <div><Typography.Text type="secondary">置信度</Typography.Text><Typography.Text>{workspaceSetupDraftConfidenceLabel(draftSession?.draft?.confidence)}</Typography.Text></div>
            </div>
            {draftSession?.draft?.generationSummary || draftSession?.draft?.warnings?.length || draftSession?.draft?.clarifyingQuestions?.length ? <div className="workspace-draft-generation-status">
              {draftSession?.draft?.generationSummary ? <Typography.Paragraph className="workspace-draft-generation-summary">{draftSession.draft.generationSummary}</Typography.Paragraph> : null}
              {hasWorkspaceSetupDraftInfoItems(draftSession?.draft?.clarifyingQuestions) ? <div className="workspace-draft-warning-list"><Typography.Text strong>待澄清问题</Typography.Text>{workspaceSetupDraftInfoList(draftSession?.draft?.clarifyingQuestions)}</div> : null}
              {hasWorkspaceSetupDraftInfoItems(draftSession?.draft?.warnings) ? <div className="workspace-draft-warning-list"><Typography.Text strong>注意事项</Typography.Text>{workspaceSetupDraftInfoList(draftSession?.draft?.warnings)}</div> : null}
            </div> : null}
            <div className="workspace-draft-grid">
              <div className="workspace-draft-panel workspace-draft-dialog-panel">
                <Typography.Text strong>需求对话</Typography.Text>
                <Input.TextArea rows={6} value={draftInput} onChange={(event) => setDraftInput(event.target.value)} placeholder="描述你希望这个工作空间完成什么；也可以继续补充约束、范围和验收要求" />
                <Space wrap><Button type="primary" loading={draftLoading} disabled={!canUseDraft} onClick={() => void generateOrReviseDraft()}>{draftSession ? '生成/修订草案' : '生成草案'}</Button><Button onClick={applyDraft} disabled={!workspaceSetupDraftReadyForApply(draftSession?.draft)}>应用到表单</Button></Space>
                <div className="workspace-draft-message-list">
                  {draftSession?.messages?.length ? <List size="small" dataSource={draftSession.messages.slice(-6)} renderItem={(item) => <List.Item><List.Item.Meta title={workspaceSetupDraftMessageLabel(item.role)} description={<Typography.Paragraph className="workspace-draft-message-content">{item.content || '-'}</Typography.Paragraph>} /></List.Item>} /> : <Typography.Paragraph type="secondary" className="workspace-draft-empty-text">草案生成只用于创建前澄清，不会自动创建工作空间；最终仍以表单里的 Goal 和 DoD 为准。</Typography.Paragraph>}
                </div>
              </div>
              <div className="workspace-draft-panel workspace-draft-preview-panel"><div className="workspace-draft-preview-section"><Typography.Text strong>Goal 草案</Typography.Text>{workspaceSetupDraftTextBlock(draftSession?.draft?.goalText)}</div><div className="workspace-draft-preview-section workspace-draft-preview-section-grow"><Typography.Text strong>DoD 草案</Typography.Text>{workspaceSetupDraftDoDList(draftSession?.draft?.doDItems)}</div></div>
            </div>
            {hasWorkspaceSetupDraftInfoItems(draftSession?.draft?.assumptions) ? <div className="workspace-draft-support-grid"><div className="workspace-draft-support-section"><Typography.Text strong>采用假设</Typography.Text>{workspaceSetupDraftInfoList(draftSession?.draft?.assumptions)}</div></div> : null}
          </Space>
        </ProCard>
      </div>
      <Form form={goalForm} layout="vertical" style={{ display: setupStep === 1 ? 'block' : 'none' }}>
        <Form.Item label="工作空间"><Input value={workspaceForm.getFieldValue('title') || '待创建，最终确认提交后生成'} disabled /></Form.Item>
        <Form.Item name="text" label="目标" rules={[{ required: true, whitespace: true, min: 1 }]}><Input.TextArea rows={5} placeholder="支持 Markdown 或普通文本；多条目标可换行或用分号分隔，提交后会规范化为独立目标项" /></Form.Item>
      </Form>
      <Form form={doDForm} layout="vertical" style={{ display: setupStep === 2 ? 'block' : 'none' }}>
        <Form.Item label="工作空间"><Input value={workspaceForm.getFieldValue('title') || '待创建，最终确认提交后生成'} disabled /></Form.Item>
        <Form.Item name="items" label="验收项" rules={[{ required: true }]}><Input.TextArea rows={5} placeholder={'每行一个验收标准\n示例：服务已完成本地部署并可访问'} /></Form.Item>
      </Form>
      <Form form={soulBindingsForm} layout="vertical" style={{ display: setupStep === 3 ? 'block' : 'none' }}>
        <Form.Item label="工作空间"><Input value={workspaceForm.getFieldValue('title') || '待创建，最终确认提交后生成'} disabled /></Form.Item>
        <Typography.Paragraph type="secondary">可选。用于设置该工作空间默认执行人格（Soul）和各任务阶段执行人格；后续可在工作空间详情中修改。</Typography.Paragraph>
        {souls.length ? <><Form.Item name="defaultSoulId" label="工作空间默认执行人格（Soul）"><Select allowClear placeholder="未绑定时不注入默认执行人格" options={soulOptions} /></Form.Item><Space className="workorch-section"><Typography.Text type="secondary">阶段执行人格绑定</Typography.Text><Button size="small" onClick={() => soulBindingsForm.setFieldsValue({ bindings: stageBindings(defaultSoulBindingStages, soulBindingsForm.getFieldValue('defaultSoulId')) })}>默认执行人格应用到全部阶段</Button></Space><div className="soul-binding-grid">{defaultSoulBindingStages.map((stage) => <Form.Item key={stage} name={['bindings', stage]} label={soulBindingStageLabel(stage)}><Select allowClear placeholder="继承默认执行人格" options={soulOptions} /></Form.Item>)}</div></> : <Empty description="暂无执行人格，可先到系统配置创建，或直接完成后续配置" />}
      </Form>
      <div style={{ display: setupStep === 4 ? 'block' : 'none' }}>
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Typography.Paragraph type="secondary">请确认以下信息。点击“确认提交”后，workorch 会按顺序创建工作空间（Workspace）、写入目标（Goal）、写入验收标准（DoD），并保存工作空间执行人格（Soul）配置。</Typography.Paragraph>
          <Descriptions bordered column={1} size="small" items={[
            { key: 'title', label: '工作空间', children: workspaceForm.getFieldValue('title') || '-' },
            { key: 'description', label: '描述', children: boundedParagraph(workspaceForm.getFieldValue('description'), { rows: 2 }) },
            { key: 'deliverableDirectory', label: '工作目录', children: workspaceForm.getFieldValue('deliverableDirectory') || '自动生成' },
            { key: 'goal', label: '目标', children: boundedParagraph(goalForm.getFieldValue('text'), { rows: 3 }) },
            { key: 'dod', label: '验收标准', children: compactTags(setupDoDItems(doDForm.getFieldValue('items')), 6) },
            { key: 'defaultSoul', label: '默认执行人格', children: soulBindingsForm.getFieldValue('defaultSoulId') ? soulNameById.get(soulBindingsForm.getFieldValue('defaultSoulId')) || soulBindingsForm.getFieldValue('defaultSoulId') : '未配置' },
            { key: 'stageSouls', label: '阶段执行人格', children: compactTags(Object.entries(soulBindingsForm.getFieldValue('bindings') ?? {}).filter(([, soulId]) => Boolean(soulId)).map(([stage, soulId]) => `${soulBindingStageLabel(stage)}: ${soulNameById.get(String(soulId)) || soulId}`), 6) }
          ]} />
        </Space>
      </div>
    </Modal>
  );
}
