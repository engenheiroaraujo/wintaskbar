# Política de segurança

## Versões com suporte

O projeto está na série **0.1.x**. Correções de segurança são feitas, em melhor
esforço, na versão mais recente.

## Como relatar uma vulnerabilidade

**Não abra uma issue pública com detalhes de uma vulnerabilidade.**

Use o relato privado do GitHub: na aba **Security** do repositório, escolha
**Report a vulnerability**. Se essa opção não estiver disponível, abra uma issue
pedindo um canal privado, **sem** descrever o problema.

Inclua, se possível:

- a versão do wintaskbar e do Windows (build);
- passos para reproduzir e o impacto que você observou;
- se o problema exige acesso local, administrador ou interação do usuário.

Este é um projeto mantido por uma pessoa, sem prazo garantido de resposta. A
meta é confirmar o recebimento e combinar a divulgação de forma coordenada.

## O que é relevante

O wintaskbar roda na sessão do usuário, sem administrador, e a única parte
carregada em outro processo é a `tap.dll`, que o Agent anexa ao `explorer.exe`.
Por isso, são de interesse, por exemplo:

- carregar uma `tap.dll` que não seja a do projeto (a DLL é procurada na mesma
  pasta do executável: mantenha essa pasta com permissão de escrita apenas para
  o seu usuário);
- falhas na leitura do tema (`theme.yaml`) ou do contrato compilado
  (`theme.compiled`) que permitam executar código ou corromper o processo;
- a tarefa agendada de logon e as chaves de registro que o aplicativo cria;
- o instalador e o desinstalador.

## O que não é

- Mudanças de comportamento causadas por atualizações do Windows: a API de
  diagnóstico XAML não é documentada e a árvore de elementos pode mudar.
- Avisos de antivírus ou do SmartScreen sobre binários sem assinatura de código.
- Problemas que exijam que o atacante já tenha controle da conta do usuário.

Veja também `THIRD_PARTY_NOTICES.md` para as bibliotecas de terceiros usadas.
